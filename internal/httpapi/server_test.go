package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ekbda/internal/access"
	"ekbda/internal/agentquality"
	"ekbda/internal/agenttask"
	"ekbda/internal/answer"
	"ekbda/internal/auth"
	"ekbda/internal/embedding"
	"ekbda/internal/evaluation"
	"ekbda/internal/ingestion"
	"ekbda/internal/initiative"
	"ekbda/internal/knowledge"
	"ekbda/internal/planning"
	"ekbda/internal/repositorysync"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

type fixedAuthenticator struct {
	identity auth.Identity
	err      error
}

func (a fixedAuthenticator) Authenticate(*http.Request) (auth.Identity, error) {
	return a.identity, a.err
}

func (fixedAuthenticator) Mode() string { return "jwt" }

func newTestHandler(t *testing.T, importRoot string) http.Handler {
	return newTestHandlerWithAccess(t, importRoot, newAccessService(t, access.ModeDisabled))
}

func newTestHandlerWithAccess(t *testing.T, importRoot string, accessService *access.Service) http.Handler {
	t.Helper()
	service := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	answerService := answer.NewService(service, answer.NewLocalExtractive(), answer.NewMemoryTraceStore())
	evaluationService := evaluation.NewService(evaluation.NewRunner(answerService), evaluation.NewMemoryStore())
	standardsService := standards.NewService(standards.NewMemoryStore())
	workspaceService, err := workspace.New("", standardsService, workspace.NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace service: %v", err)
	}
	t.Cleanup(evaluationService.Close)
	repositorySyncService := repositorysync.New(workspaceService, service, repositorysync.NewMemoryStore())
	planningService := planning.NewService(planning.NewMemoryStore(), planning.NewLocalProvider(), service, standardsService, workspaceService, repositorySyncService)
	initiativeService := initiative.NewService(initiative.NewMemoryStore(), initiative.NewLocalProvider(), planningService)
	return New(service, ingestion.New(importRoot, service, ingestion.NewMemoryJobStore()), answerService, evaluationService, standardsService, workspaceService, accessService, repositorySyncService, planningService, initiativeService)
}

func newAccessService(t *testing.T, mode string) *access.Service {
	t.Helper()
	service, err := access.New(access.NewMemoryStore(), mode)
	if err != nil {
		t.Fatalf("create access service: %v", err)
	}
	return service
}

func TestProjectAccessPolicyLifecycleAndEnforcement(t *testing.T) {
	accessService := newAccessService(t, access.ModeEnforced)
	handler := newTestHandlerWithAccess(t, "", accessService)

	publish := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/access/projects", strings.NewReader(body))
		request.Header.Set("X-User-ID", "admin-1")
		request.Header.Set("X-User-Roles", "knowledge_admin")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	first := publish(`{
		"project":"order-service","owner":"platform","users":["developer-1"],
		"roles":["team_order"],"repositories":["services/order"]
	}`)
	if first.Code != http.StatusCreated || !strings.Contains(first.Body.String(), `"version":1`) {
		t.Fatalf("publish first policy: %d %s", first.Code, first.Body.String())
	}
	second := publish(`{
		"project":"order-service","owner":"platform","users":["developer-1"],
		"roles":["team_order","release_engineer"],"repositories":["services/order"]
	}`)
	if second.Code != http.StatusCreated || !strings.Contains(second.Body.String(), `"version":2`) {
		t.Fatalf("publish second policy: %d %s", second.Code, second.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/v1/access/projects/order-service", nil)
	get.Header.Set("X-User-ID", "admin-1")
	get.Header.Set("X-User-Roles", "knowledge_admin")
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"version":2`) {
		t.Fatalf("get active policy: %d %s", getResponse.Code, getResponse.Body.String())
	}

	history := httptest.NewRequest(http.MethodGet, "/api/v1/access/projects/order-service/versions", nil)
	history.Header.Set("X-User-ID", "admin-1")
	history.Header.Set("X-User-Roles", "knowledge_admin")
	historyResponse := httptest.NewRecorder()
	handler.ServeHTTP(historyResponse, history)
	if historyResponse.Code != http.StatusOK || !strings.Contains(historyResponse.Body.String(), `"version":1`) || !strings.Contains(historyResponse.Body.String(), `"version":2`) {
		t.Fatalf("get policy history: %d %s", historyResponse.Code, historyResponse.Body.String())
	}

	allowed := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/search?q=start&project=order-service", nil)
	allowed.Header.Set("X-User-ID", "role-member")
	allowed.Header.Set("X-User-Roles", "TEAM_ORDER")
	allowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Code != http.StatusOK {
		t.Fatalf("role member search: %d %s", allowedResponse.Code, allowedResponse.Body.String())
	}

	for _, target := range []string{
		"/api/v1/knowledge/search?q=start&project=payment-service",
		"/api/v1/knowledge/search?q=start",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("X-User-ID", "developer-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || response.Body.String() != "{\"error\":\"project access denied\"}\n" {
			t.Fatalf("request %s must be denied without policy disclosure: %d %s", target, response.Code, response.Body.String())
		}
	}

	deniedPosts := []struct {
		path string
		body string
	}{
		{"/api/v1/knowledge/answers", `{"query":"how to start","project":"payment-service"}`},
		{"/api/v1/standards/validations", `{"project":"payment-service","technology":"go","files":[{"path":"README.md"}]}`},
	}
	for _, denied := range deniedPosts {
		request := httptest.NewRequest(http.MethodPost, denied.path, strings.NewReader(denied.body))
		request.Header.Set("X-User-ID", "developer-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || response.Body.String() != "{\"error\":\"project access denied\"}\n" {
			t.Fatalf("request %s must be denied before business processing: %d %s", denied.path, response.Code, response.Body.String())
		}
	}
}

func TestWorkspaceAuthorizationChecksRepositoryBeforeScanning(t *testing.T) {
	accessService := newAccessService(t, access.ModeEnforced)
	_, err := accessService.CreatePolicy(context.Background(), access.CreatePolicyInput{
		Project: "order-service", Owner: "platform", Users: []string{"developer-1"},
		Repositories: []string{"services/order"},
	}, "admin-1")
	if err != nil {
		t.Fatalf("seed access policy: %v", err)
	}
	handler := newTestHandlerWithAccess(t, "", accessService)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/validations", strings.NewReader(`{
		"repository":"services/payment","project":"order-service","technology":"go"
	}`))
	request.Header.Set("X-User-ID", "developer-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unmapped repository must be denied: %d %s", response.Code, response.Body.String())
	}
	syncRequest := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/syncs", strings.NewReader(`{
		"repository":"services/payment","project":"order-service"
	}`))
	syncRequest.Header.Set("X-User-ID", "developer-1")
	syncResponse := httptest.NewRecorder()
	handler.ServeHTTP(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusForbidden {
		t.Fatalf("unmapped repository sync must be denied: %d %s", syncResponse.Code, syncResponse.Body.String())
	}
	planningRequest := httptest.NewRequest(http.MethodPost, "/api/v1/planning/sessions", strings.NewReader(`{
		"repository":"services/payment","project":"order-service","technology":"go",
		"title":"Payment change","requirement":"Implement a payment change with documented acceptance criteria"
	}`))
	planningRequest.Header.Set("X-User-ID", "developer-1")
	planningResponse := httptest.NewRecorder()
	handler.ServeHTTP(planningResponse, planningRequest)
	if planningResponse.Code != http.StatusForbidden {
		t.Fatalf("unmapped planning repository must be denied: %d %s", planningResponse.Code, planningResponse.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/validations", strings.NewReader(`{
		"repository":"services/order","project":"order-service","technology":"go"
	}`))
	request.Header.Set("X-User-ID", "developer-1")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("mapped repository should reach disabled workspace service: %d %s", response.Code, response.Body.String())
	}
	planningRequest = httptest.NewRequest(http.MethodPost, "/api/v1/planning/sessions", strings.NewReader(`{
		"repository":"services/order","project":"order-service","technology":"go",
		"title":"Order change","requirement":"Implement an order change with documented acceptance criteria"
	}`))
	planningRequest.Header.Set("X-User-ID", "developer-1")
	planningResponse = httptest.NewRecorder()
	handler.ServeHTTP(planningResponse, planningRequest)
	if planningResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("mapped planning repository should reach disabled workspace service: %d %s", planningResponse.Code, planningResponse.Body.String())
	}
}

func TestImportDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# 企业知识\n\n新人启动说明"), 0o600); err != nil {
		t.Fatalf("write import fixture: %v", err)
	}
	handler := newTestHandler(t, root)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/imports", strings.NewReader(`{
		"path":".","project":"onboarding","business_domain":"研发","classification":"internal"
	}`))
	request.Header.Set("X-User-ID", "admin-1")
	request.Header.Set("X-User-Roles", "knowledge_admin")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	var started ingestion.Report
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode started job: %v", err)
	}

	var completed ingestion.Report
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/imports/"+started.ID, nil)
		statusRequest.Header.Set("X-User-ID", "admin-1")
		statusRequest.Header.Set("X-User-Roles", "knowledge_admin")
		statusResponse := httptest.NewRecorder()
		handler.ServeHTTP(statusResponse, statusRequest)
		if statusResponse.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", statusResponse.Code)
		}
		if err := json.Unmarshal(statusResponse.Body.Bytes(), &completed); err != nil {
			t.Fatalf("decode job status: %v", err)
		}
		if completed.Status == "completed" || completed.Status == "completed_with_errors" || completed.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed.Status != "completed" || completed.Created != 1 {
		t.Fatalf("unexpected completed job: %#v", completed)
	}
	versionsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/documents/"+completed.Files[0].DocumentID+"/versions", nil)
	versionsRequest.Header.Set("X-User-ID", "admin-1")
	versionsRequest.Header.Set("X-User-Roles", "knowledge_admin")
	versionsResponse := httptest.NewRecorder()
	handler.ServeHTTP(versionsResponse, versionsRequest)
	if versionsResponse.Code != http.StatusOK || !strings.Contains(versionsResponse.Body.String(), `"version":1`) {
		t.Fatalf("unexpected versions response: %d %s", versionsResponse.Code, versionsResponse.Body.String())
	}
}

func TestHealth(t *testing.T) {
	handler := newTestHandler(t, "")
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestDocumentCreationRequiresAdminRole(t *testing.T) {
	handler := newTestHandler(t, "")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/documents", strings.NewReader(`{
		"title":"项目说明","content":"项目内容","source_uri":"git://project/README.md","project":"project"
	}`))
	request.Header.Set("X-User-ID", "user-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestVerifiedIdentityCannotBeEscalatedByDevelopmentHeaders(t *testing.T) {
	service := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	answerService := answer.NewService(service, answer.NewLocalExtractive(), answer.NewMemoryTraceStore())
	evaluationService := evaluation.NewService(evaluation.NewRunner(answerService), evaluation.NewMemoryStore())
	standardsService := standards.NewService(standards.NewMemoryStore())
	workspaceService, err := workspace.New("", standardsService, workspace.NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace service: %v", err)
	}
	repositorySyncService := repositorysync.New(workspaceService, service, repositorysync.NewMemoryStore())
	planningService := planning.NewService(planning.NewMemoryStore(), planning.NewLocalProvider(), service, standardsService, workspaceService, repositorySyncService)
	initiativeService := initiative.NewService(initiative.NewMemoryStore(), initiative.NewLocalProvider(), planningService)
	t.Cleanup(evaluationService.Close)
	handler := New(
		service,
		ingestion.New("", service, ingestion.NewMemoryJobStore()),
		answerService,
		evaluationService,
		standardsService,
		workspaceService,
		newAccessService(t, access.ModeDisabled),
		repositorySyncService,
		planningService,
		initiativeService,
		fixedAuthenticator{identity: auth.Identity{UserID: "verified-developer", Roles: []string{"developer"}, Source: "jwt"}},
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/documents", strings.NewReader(`{
		"title":"Project","content":"Content","source_uri":"git://project/README.md","project":"project"
	}`))
	request.Header.Set("X-User-ID", "spoofed-admin")
	request.Header.Set("X-User-Roles", "knowledge_admin")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("development headers escalated verified identity: %d %s", response.Code, response.Body.String())
	}
}

func TestJWTAuthenticationFailureReturnsBearerChallenge(t *testing.T) {
	service := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	answerService := answer.NewService(service, answer.NewLocalExtractive(), answer.NewMemoryTraceStore())
	evaluationService := evaluation.NewService(evaluation.NewRunner(answerService), evaluation.NewMemoryStore())
	standardsService := standards.NewService(standards.NewMemoryStore())
	workspaceService, err := workspace.New("", standardsService, workspace.NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace service: %v", err)
	}
	repositorySyncService := repositorysync.New(workspaceService, service, repositorysync.NewMemoryStore())
	planningService := planning.NewService(planning.NewMemoryStore(), planning.NewLocalProvider(), service, standardsService, workspaceService, repositorySyncService)
	initiativeService := initiative.NewService(initiative.NewMemoryStore(), initiative.NewLocalProvider(), planningService)
	t.Cleanup(evaluationService.Close)
	handler := New(
		service,
		ingestion.New("", service, ingestion.NewMemoryJobStore()),
		answerService,
		evaluationService,
		standardsService,
		workspaceService,
		newAccessService(t, access.ModeDisabled),
		repositorySyncService,
		planningService,
		initiativeService,
		fixedAuthenticator{err: auth.ErrUnauthenticated},
	)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/search?q=test", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unexpected authentication response: %d %#v", response.Code, response.Header())
	}
}

func TestCreateThenSearch(t *testing.T) {
	handler := newTestHandler(t, "")
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/documents", strings.NewReader(`{
		"title":"订单服务启动说明","content":"运行 go run ./cmd/server 启动订单服务","source_uri":"git://order/README.md","project":"order"
	}`))
	createRequest.Header.Set("X-User-ID", "admin-1")
	createRequest.Header.Set("X-User-Roles", "knowledge_admin")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createResponse.Code, createResponse.Body.String())
	}

	searchRequest := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/search?q=启动&project=order", nil)
	searchRequest.Header.Set("X-User-ID", "developer-1")
	searchRequest.Header.Set("X-User-Roles", "developer")
	searchResponse := httptest.NewRecorder()
	handler.ServeHTTP(searchResponse, searchRequest)
	if searchResponse.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", searchResponse.Code)
	}
	if !strings.Contains(searchResponse.Body.String(), "git://order/README.md") {
		t.Fatalf("expected citation in response: %s", searchResponse.Body.String())
	}
}

func TestStandardPackageAndValidationLifecycle(t *testing.T) {
	handler := newTestHandler(t, "")
	publish := httptest.NewRequest(http.MethodPost, "/api/v1/standards/packages", strings.NewReader(`{
		"name":"go-service","scope":"technology","selector":"go","owner":"platform-team",
		"rules":[{
			"id":"unit-test-required","title":"Unit tests required","category":"testing","level":"block",
			"check":{"type":"minimum_matches","target":"_test\\.go$","minimum":1}
		}]
	}`))
	publish.Header.Set("X-User-ID", "admin-1")
	publish.Header.Set("X-User-Roles", "knowledge_admin")
	publishResponse := httptest.NewRecorder()
	handler.ServeHTTP(publishResponse, publish)
	if publishResponse.Code != http.StatusCreated {
		t.Fatalf("publish standard package: %d %s", publishResponse.Code, publishResponse.Body.String())
	}
	var standard standards.Package
	if err := json.Unmarshal(publishResponse.Body.Bytes(), &standard); err != nil {
		t.Fatalf("decode standard package: %v", err)
	}
	if standard.Version != 1 || standard.CreatedBy != "admin-1" || standard.DefinitionHash == "" {
		t.Fatalf("unexpected standard package: %#v", standard)
	}

	validation := httptest.NewRequest(http.MethodPost, "/api/v1/standards/validations", strings.NewReader(`{
		"project":"order-service","technology":"go",
		"files":[{"path":"README.md","content":"# Order service"}]
	}`))
	validation.Header.Set("X-User-ID", "developer-1")
	validation.Header.Set("X-User-Roles", "developer")
	validationResponse := httptest.NewRecorder()
	handler.ServeHTTP(validationResponse, validation)
	if validationResponse.Code != http.StatusOK {
		t.Fatalf("validate standards: %d %s", validationResponse.Code, validationResponse.Body.String())
	}
	var report standards.ValidationReport
	if err := json.Unmarshal(validationResponse.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode validation report: %v", err)
	}
	if report.Passed || report.BlockingCount != 1 || report.ValidatedBy != "developer-1" || len(report.Packages) != 1 {
		t.Fatalf("unexpected validation report: %#v", report)
	}

	getReport := httptest.NewRequest(http.MethodGet, "/api/v1/standards/validations/"+report.ID, nil)
	getReport.Header.Set("X-User-ID", "admin-1")
	getReport.Header.Set("X-User-Roles", "knowledge_admin")
	getReportResponse := httptest.NewRecorder()
	handler.ServeHTTP(getReportResponse, getReport)
	if getReportResponse.Code != http.StatusOK || !strings.Contains(getReportResponse.Body.String(), `"input_hash"`) {
		t.Fatalf("get standard report: %d %s", getReportResponse.Code, getReportResponse.Body.String())
	}
}

func TestStandardPackagePublicationRequiresAdminRole(t *testing.T) {
	handler := newTestHandler(t, "")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/standards/packages", strings.NewReader(`{}`))
	request.Header.Set("X-User-ID", "developer-1")
	request.Header.Set("X-User-Roles", "developer")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkspaceValidationLifecycle(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "app")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	for _, arguments := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "HTTP Test"},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# App\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	command := exec.Command("git", "add", ".")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	command = exec.Command("git", "commit", "--no-gpg-sign", "-m", "initial")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}

	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	answerService := answer.NewService(knowledgeService, answer.NewLocalExtractive(), answer.NewMemoryTraceStore())
	evaluationService := evaluation.NewService(evaluation.NewRunner(answerService), evaluation.NewMemoryStore())
	t.Cleanup(evaluationService.Close)
	standardsService := standards.NewService(standards.NewMemoryStore())
	_, err := standardsService.CreatePackage(context.Background(), standards.CreatePackageInput{
		Name: "app-layout", Scope: standards.ScopeProject, Selector: "app", Owner: "platform",
		Rules: []standards.Rule{{ID: "readme", Title: "README", Category: standards.CategoryDirectory, Level: standards.LevelBlock, Check: &standards.RuleCheck{Type: standards.CheckRequiredPath, Pattern: "README.md"}}},
	}, "admin-1")
	if err != nil {
		t.Fatalf("create standards: %v", err)
	}
	workspaceService, err := workspace.New(root, standardsService, workspace.NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace service: %v", err)
	}
	repositorySyncService := repositorysync.New(workspaceService, knowledgeService, repositorysync.NewMemoryStore())
	planningService := planning.NewService(planning.NewMemoryStore(), planning.NewLocalProvider(), knowledgeService, standardsService, workspaceService, repositorySyncService)
	initiativeService := initiative.NewService(initiative.NewMemoryStore(), initiative.NewLocalProvider(), planningService)
	agentTaskService := agenttask.NewService(agenttask.NewMemoryStore(), agenttask.Pricing{}, time.Second)
	if err := agentTaskService.Register(agenttask.KindRoleReview, func(ctx context.Context, task agenttask.Task) (agenttask.ExecutionResult, error) {
		var input agenttask.RoleReviewInput
		if err := json.Unmarshal(task.Input, &input); err != nil {
			return agenttask.ExecutionResult{}, err
		}
		session, err := planningService.SubmitRoleReviews(ctx, input.SessionID, input.Revision, task.TriggeredBy, input.Roles, input.GovernanceOverride)
		if err != nil {
			return agenttask.ExecutionResult{}, err
		}
		return agenttask.ExecutionResult{ResourceID: session.ID, Quality: agentquality.RoleReview(session)}, nil
	}); err != nil {
		t.Fatalf("register role-review task: %v", err)
	}
	if err := agentTaskService.Register(agenttask.KindProjectPackage, func(ctx context.Context, task agenttask.Task) (agenttask.ExecutionResult, error) {
		var input agenttask.ProjectPackageInput
		if err := json.Unmarshal(task.Input, &input); err != nil {
			return agenttask.ExecutionResult{}, err
		}
		projectPackage, err := initiativeService.Create(ctx, initiative.CreateInput{SessionID: input.SessionID, Name: input.Name, ChangeSummary: input.ChangeSummary}, task.TriggeredBy)
		if err != nil {
			return agenttask.ExecutionResult{}, err
		}
		return agenttask.ExecutionResult{ResourceID: projectPackage.ID, Quality: agentquality.ProjectPackage(projectPackage)}, nil
	}); err != nil {
		t.Fatalf("register project-package task: %v", err)
	}
	t.Cleanup(agentTaskService.Close)
	handler := NewWithAgentTasks(
		knowledgeService, ingestion.New("", knowledgeService, ingestion.NewMemoryJobStore()),
		answerService, evaluationService, standardsService, workspaceService, newAccessService(t, access.ModeDisabled),
		repositorySyncService,
		planningService,
		initiativeService,
		agentTaskService,
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/validations", strings.NewReader(`{
		"repository":"app","project":"app","technology":"go"
	}`))
	request.Header.Set("X-User-ID", "developer-1")
	request.Header.Set("X-User-Roles", "developer")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("validate workspace: %d %s", response.Code, response.Body.String())
	}
	var result workspace.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode workspace result: %v", err)
	}
	if !result.Repository.Passed || result.Repository.ValidatedBy != "developer-1" || result.Repository.HeadCommit == "" || result.Standards.RuleCount != 1 {
		t.Fatalf("unexpected workspace result: %#v", result)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/validations/"+result.Repository.ID, nil)
	getRequest.Header.Set("X-User-ID", "admin-1")
	getRequest.Header.Set("X-User-Roles", "knowledge_admin")
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), result.Repository.HeadCommit) {
		t.Fatalf("get workspace snapshot: %d %s", getResponse.Code, getResponse.Body.String())
	}

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/syncs", strings.NewReader(`{
		"repository":"app","project":"app","business_domain":"engineering","classification":"internal"
	}`))
	syncRequest.Header.Set("X-User-ID", "developer-1")
	syncResponse := httptest.NewRecorder()
	handler.ServeHTTP(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync repository knowledge: %d %s", syncResponse.Code, syncResponse.Body.String())
	}
	var syncReport repositorysync.Report
	if err := json.Unmarshal(syncResponse.Body.Bytes(), &syncReport); err != nil {
		t.Fatalf("decode repository sync report: %v", err)
	}
	if syncReport.Created != 1 || syncReport.HeadCommit != result.Repository.HeadCommit || syncReport.SyncedBy != "developer-1" {
		t.Fatalf("unexpected repository sync report: %#v", syncReport)
	}

	syncGetRequest := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/syncs/"+syncReport.ID, nil)
	syncGetRequest.Header.Set("X-User-ID", "admin-1")
	syncGetRequest.Header.Set("X-User-Roles", "knowledge_admin")
	syncGetResponse := httptest.NewRecorder()
	handler.ServeHTTP(syncGetResponse, syncGetRequest)
	if syncGetResponse.Code != http.StatusOK || !strings.Contains(syncGetResponse.Body.String(), syncReport.HeadCommit) {
		t.Fatalf("get repository sync report: %d %s", syncGetResponse.Code, syncGetResponse.Body.String())
	}

	planningRequest := httptest.NewRequest(http.MethodPost, "/api/v1/planning/sessions", strings.NewReader(`{
		"project":"app","repository":"app","technology":"go","title":"增加订单导出能力",
		"requirement":"为 App 增加订单导出接口，并确保实现遵守当前项目规范和发布流程"
	}`))
	planningRequest.Header.Set("X-User-ID", "developer-1")
	planningRequest.Header.Set("X-User-Roles", "developer")
	planningResponse := httptest.NewRecorder()
	handler.ServeHTTP(planningResponse, planningRequest)
	if planningResponse.Code != http.StatusCreated || strings.Contains(planningResponse.Body.String(), `"snippet":`) {
		t.Fatalf("create safe planning session: %d %s", planningResponse.Code, planningResponse.Body.String())
	}
	var planningSession planning.Session
	if err := json.Unmarshal(planningResponse.Body.Bytes(), &planningSession); err != nil {
		t.Fatalf("decode planning session: %v", err)
	}
	if planningSession.Status != planning.StatusAwaitingClarification || planningSession.Revision != 1 || len(planningSession.Questions) != 3 {
		t.Fatalf("unexpected planning session: %#v", planningSession)
	}

	otherClarification := httptest.NewRequest(http.MethodPost, "/api/v1/planning/sessions/"+planningSession.ID+"/clarifications", strings.NewReader(`{
		"revision":1,"answers":[
			{"question_id":"acceptance","answer":"接口返回可下载文件并通过集成测试"},
			{"question_id":"constraints","answer":"兼容现有 API，禁止停机迁移"},
			{"question_id":"scope","answer":"不包含管理后台页面"}
		]
	}`))
	otherClarification.Header.Set("X-User-ID", "developer-2")
	otherClarification.Header.Set("X-User-Roles", "developer")
	otherClarificationResponse := httptest.NewRecorder()
	handler.ServeHTTP(otherClarificationResponse, otherClarification)
	if otherClarificationResponse.Code != http.StatusForbidden {
		t.Fatalf("other user submitted clarifications: %d %s", otherClarificationResponse.Code, otherClarificationResponse.Body.String())
	}

	clarification := httptest.NewRequest(http.MethodPost, "/api/v1/planning/sessions/"+planningSession.ID+"/clarifications", strings.NewReader(`{
		"revision":1,"answers":[
			{"question_id":"acceptance","answer":"接口返回可下载文件并通过集成测试"},
			{"question_id":"constraints","answer":"兼容现有 API，禁止停机迁移"},
			{"question_id":"scope","answer":"不包含管理后台页面"}
		]
	}`))
	clarification.Header.Set("X-User-ID", "developer-1")
	clarification.Header.Set("X-User-Roles", "developer")
	clarificationResponse := httptest.NewRecorder()
	handler.ServeHTTP(clarificationResponse, clarification)
	if clarificationResponse.Code != http.StatusOK {
		t.Fatalf("submit planning clarifications: %d %s", clarificationResponse.Code, clarificationResponse.Body.String())
	}
	if err := json.Unmarshal(clarificationResponse.Body.Bytes(), &planningSession); err != nil {
		t.Fatalf("decode planned session: %v", err)
	}
	if planningSession.Status != planning.StatusAwaitingRoleReview || planningSession.Revision != 2 || planningSession.Plan == nil || len(planningSession.Plan.Steps) != 4 {
		t.Fatalf("unexpected generated plan: %#v", planningSession)
	}

	roleReviewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/agent-tasks/role-reviews", strings.NewReader(`{"session_id":"`+planningSession.ID+`","revision":2}`))
	roleReviewRequest.Header.Set("X-User-ID", "developer-1")
	roleReviewRequest.Header.Set("X-User-Roles", "developer")
	roleReviewResponse := httptest.NewRecorder()
	handler.ServeHTTP(roleReviewResponse, roleReviewRequest)
	if roleReviewResponse.Code != http.StatusAccepted {
		t.Fatalf("create role-review task: %d %s", roleReviewResponse.Code, roleReviewResponse.Body.String())
	}
	var roleReviewTask agenttask.Task
	if err := json.Unmarshal(roleReviewResponse.Body.Bytes(), &roleReviewTask); err != nil {
		t.Fatalf("decode role-review task: %v", err)
	}
	roleReviewTask = waitForHTTPAgentTask(t, handler, roleReviewTask.ID, "developer-1", "developer")
	if roleReviewTask.Status != agenttask.StatusCompleted || roleReviewTask.ResourceID != planningSession.ID || !roleReviewTask.Quality.Passed {
		t.Fatalf("unexpected role-review task: %#v", roleReviewTask)
	}
	roleReviewSessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/planning/sessions/"+planningSession.ID, nil)
	roleReviewSessionRequest.Header.Set("X-User-ID", "developer-1")
	roleReviewSessionResponse := httptest.NewRecorder()
	handler.ServeHTTP(roleReviewSessionResponse, roleReviewSessionRequest)
	if roleReviewSessionResponse.Code != http.StatusOK || json.Unmarshal(roleReviewSessionResponse.Body.Bytes(), &planningSession) != nil {
		t.Fatalf("load role-reviewed session: %d %s", roleReviewSessionResponse.Code, roleReviewSessionResponse.Body.String())
	}
	if planningSession.Status != planning.StatusAwaitingApproval || planningSession.Revision != 3 || planningSession.RoleReview == nil || len(planningSession.RoleReview.Reviews) != len(planning.RequiredReviewRoles()) {
		t.Fatalf("unexpected role review session: %#v", planningSession)
	}

	selfDecision := httptest.NewRequest(http.MethodPost, "/api/v1/planning/sessions/"+planningSession.ID+"/decision", strings.NewReader(`{"revision":3,"decision":"approve","reason":"looks good"}`))
	selfDecision.Header.Set("X-User-ID", "developer-1")
	selfDecision.Header.Set("X-User-Roles", "project_approver")
	selfDecisionResponse := httptest.NewRecorder()
	handler.ServeHTTP(selfDecisionResponse, selfDecision)
	if selfDecisionResponse.Code != http.StatusForbidden {
		t.Fatalf("creator self-approved plan: %d %s", selfDecisionResponse.Code, selfDecisionResponse.Body.String())
	}

	decision := httptest.NewRequest(http.MethodPost, "/api/v1/planning/sessions/"+planningSession.ID+"/decision", strings.NewReader(`{"revision":3,"decision":"approve","reason":"范围和验收标准已确认"}`))
	decision.Header.Set("X-User-ID", "approver-1")
	decision.Header.Set("X-User-Roles", "project_approver")
	decisionResponse := httptest.NewRecorder()
	handler.ServeHTTP(decisionResponse, decision)
	if decisionResponse.Code != http.StatusOK {
		t.Fatalf("approve planning session: %d %s", decisionResponse.Code, decisionResponse.Body.String())
	}
	if err := json.Unmarshal(decisionResponse.Body.Bytes(), &planningSession); err != nil {
		t.Fatalf("decode approved session: %v", err)
	}
	if planningSession.Status != planning.StatusApproved || planningSession.Revision != 4 || planningSession.ReviewedBy != "approver-1" {
		t.Fatalf("unexpected approved session: %#v", planningSession)
	}

	packageBody := `{"session_id":"` + planningSession.ID + `","name":"order-export","change_summary":"初始立项包"}`
	unauthorizedPackageRequest := httptest.NewRequest(http.MethodPost, "/api/v1/project-packages", strings.NewReader(packageBody))
	unauthorizedPackageRequest.Header.Set("X-User-ID", "developer-1")
	unauthorizedPackageRequest.Header.Set("X-User-Roles", "developer")
	unauthorizedPackageResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedPackageResponse, unauthorizedPackageRequest)
	if unauthorizedPackageResponse.Code != http.StatusForbidden {
		t.Fatalf("developer published project package: %d %s", unauthorizedPackageResponse.Code, unauthorizedPackageResponse.Body.String())
	}

	packageRequest := httptest.NewRequest(http.MethodPost, "/api/v1/project-packages", strings.NewReader(packageBody))
	packageRequest.Header.Set("X-User-ID", "approver-1")
	packageRequest.Header.Set("X-User-Roles", "project_approver")
	packageResponse := httptest.NewRecorder()
	handler.ServeHTTP(packageResponse, packageRequest)
	if packageResponse.Code != http.StatusCreated {
		t.Fatalf("create project package: %d %s", packageResponse.Code, packageResponse.Body.String())
	}
	var projectPackage initiative.Package
	if err := json.Unmarshal(packageResponse.Body.Bytes(), &projectPackage); err != nil {
		t.Fatalf("decode project package: %v", err)
	}
	if projectPackage.Version != 1 || len(projectPackage.Artifacts) != len(initiative.RequiredArtifactTypes()) || projectPackage.Source.PlanningSessionID != planningSession.ID {
		t.Fatalf("unexpected project package: %#v", projectPackage)
	}

	packageTaskRequest := httptest.NewRequest(http.MethodPost, "/api/v1/agent-tasks/project-packages", strings.NewReader(`{"session_id":"`+planningSession.ID+`","name":"order-export","change_summary":"异步生成第二版"}`))
	packageTaskRequest.Header.Set("X-User-ID", "approver-1")
	packageTaskRequest.Header.Set("X-User-Roles", "project_approver")
	packageTaskResponse := httptest.NewRecorder()
	handler.ServeHTTP(packageTaskResponse, packageTaskRequest)
	if packageTaskResponse.Code != http.StatusAccepted {
		t.Fatalf("create project-package task: %d %s", packageTaskResponse.Code, packageTaskResponse.Body.String())
	}
	var packageTask agenttask.Task
	if err := json.Unmarshal(packageTaskResponse.Body.Bytes(), &packageTask); err != nil {
		t.Fatalf("decode project-package task: %v", err)
	}
	packageTask = waitForHTTPAgentTask(t, handler, packageTask.ID, "developer-1", "developer")
	if packageTask.Status != agenttask.StatusCompleted || packageTask.ResourceID == "" || !packageTask.Quality.Passed {
		t.Fatalf("unexpected project-package task: %#v", packageTask)
	}

	packageGetRequest := httptest.NewRequest(http.MethodGet, "/api/v1/project-packages/"+projectPackage.ID, nil)
	packageGetRequest.Header.Set("X-User-ID", "developer-1")
	packageGetRequest.Header.Set("X-User-Roles", "developer")
	packageGetResponse := httptest.NewRecorder()
	handler.ServeHTTP(packageGetResponse, packageGetRequest)
	if packageGetResponse.Code != http.StatusOK || !strings.Contains(packageGetResponse.Body.String(), `"definition_hash"`) {
		t.Fatalf("get project package: %d %s", packageGetResponse.Code, packageGetResponse.Body.String())
	}

	packageListRequest := httptest.NewRequest(http.MethodGet, "/api/v1/project-packages?project=app&name=order-export", nil)
	packageListRequest.Header.Set("X-User-ID", "developer-1")
	packageListResponse := httptest.NewRecorder()
	handler.ServeHTTP(packageListResponse, packageListRequest)
	if packageListResponse.Code != http.StatusOK || !strings.Contains(packageListResponse.Body.String(), projectPackage.ID) {
		t.Fatalf("list project packages: %d %s", packageListResponse.Code, packageListResponse.Body.String())
	}

	reviewBody := `{"artifact_type":"prd","package_hash":"` + projectPackage.DefinitionHash + `","decision":"approve","comment":"scope and acceptance criteria confirmed"}`
	unauthorizedReviewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/project-packages/"+projectPackage.ID+"/reviews", strings.NewReader(reviewBody))
	unauthorizedReviewRequest.Header.Set("X-User-ID", "developer-1")
	unauthorizedReviewRequest.Header.Set("X-User-Roles", "developer")
	unauthorizedReviewResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedReviewResponse, unauthorizedReviewRequest)
	if unauthorizedReviewResponse.Code != http.StatusForbidden {
		t.Fatalf("developer reviewed project package: %d %s", unauthorizedReviewResponse.Code, unauthorizedReviewResponse.Body.String())
	}

	reviewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/project-packages/"+projectPackage.ID+"/reviews", strings.NewReader(reviewBody))
	reviewRequest.Header.Set("X-User-ID", "approver-1")
	reviewRequest.Header.Set("X-User-Roles", "project_approver")
	reviewResponse := httptest.NewRecorder()
	handler.ServeHTTP(reviewResponse, reviewRequest)
	if reviewResponse.Code != http.StatusCreated || !strings.Contains(reviewResponse.Body.String(), `"sequence":1`) {
		t.Fatalf("review project package: %d %s", reviewResponse.Code, reviewResponse.Body.String())
	}

	reviewsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/project-packages/"+projectPackage.ID+"/reviews?artifact_type=prd", nil)
	reviewsRequest.Header.Set("X-User-ID", "developer-1")
	reviewsResponse := httptest.NewRecorder()
	handler.ServeHTTP(reviewsResponse, reviewsRequest)
	if reviewsResponse.Code != http.StatusOK || !strings.Contains(reviewsResponse.Body.String(), `"decision":"approve"`) {
		t.Fatalf("list project package reviews: %d %s", reviewsResponse.Code, reviewsResponse.Body.String())
	}

	markdownRequest := httptest.NewRequest(http.MethodGet, "/api/v1/project-packages/"+projectPackage.ID+"/export?format=markdown", nil)
	markdownRequest.Header.Set("X-User-ID", "developer-1")
	markdownResponse := httptest.NewRecorder()
	handler.ServeHTTP(markdownResponse, markdownRequest)
	if markdownResponse.Code != http.StatusOK || !strings.HasPrefix(markdownResponse.Header().Get("Content-Type"), "text/markdown") || !strings.Contains(markdownResponse.Body.String(), "REQ-001") {
		t.Fatalf("export project package Markdown: %d %s", markdownResponse.Code, markdownResponse.Body.String())
	}

	docxRequest := httptest.NewRequest(http.MethodGet, "/api/v1/project-packages/"+projectPackage.ID+"/export?format=docx", nil)
	docxRequest.Header.Set("X-User-ID", "developer-1")
	docxResponse := httptest.NewRecorder()
	handler.ServeHTTP(docxResponse, docxRequest)
	if docxResponse.Code != http.StatusOK || docxResponse.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || !strings.HasPrefix(docxResponse.Body.String(), "PK") {
		t.Fatalf("export project package DOCX: %d content-type=%s", docxResponse.Code, docxResponse.Header().Get("Content-Type"))
	}

	eventsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/planning/sessions/"+planningSession.ID+"/events", nil)
	eventsRequest.Header.Set("X-User-ID", "developer-1")
	eventsResponse := httptest.NewRecorder()
	handler.ServeHTTP(eventsResponse, eventsRequest)
	if eventsResponse.Code != http.StatusOK {
		t.Fatalf("list planning events: %d %s", eventsResponse.Code, eventsResponse.Body.String())
	}
	var eventResult struct {
		Events []planning.Event `json:"events"`
	}
	if err := json.Unmarshal(eventsResponse.Body.Bytes(), &eventResult); err != nil || len(eventResult.Events) != 4 {
		t.Fatalf("unexpected planning events: %v %#v", err, eventResult.Events)
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/planning/sessions?project=app", nil)
	listRequest.Header.Set("X-User-ID", "developer-1")
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), planningSession.ID) {
		t.Fatalf("list planning sessions: %d %s", listResponse.Code, listResponse.Body.String())
	}
}

func waitForHTTPAgentTask(t *testing.T, handler http.Handler, id, userID, userRoles string) agenttask.Task {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/agent-tasks/"+id, nil)
		request.Header.Set("X-User-ID", userID)
		request.Header.Set("X-User-Roles", userRoles)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("get agent task: %d %s", response.Code, response.Body.String())
		}
		var task agenttask.Task
		if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
			t.Fatalf("decode agent task: %v", err)
		}
		if task.Status == agenttask.StatusCompleted || task.Status == agenttask.StatusFailed || task.Status == agenttask.StatusCanceled {
			return task
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("agent task did not finish")
	return agenttask.Task{}
}

func TestWorkspaceValidationFailsWhenRootIsNotConfigured(t *testing.T) {
	handler := newTestHandler(t, "")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/validations", strings.NewReader(`{
		"repository":"app","project":"app","technology":"go"
	}`))
	request.Header.Set("X-User-ID", "developer-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateGroundedAnswer(t *testing.T) {
	handler := newTestHandler(t, "")
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/documents", strings.NewReader(`{
		"title":"本地启动","content":"运行 go run ./cmd/server 启动服务","source_uri":"git://app/README.md","project":"app"
	}`))
	createRequest.Header.Set("X-User-ID", "admin-1")
	createRequest.Header.Set("X-User-Roles", "knowledge_admin")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create knowledge: %d %s", createResponse.Code, createResponse.Body.String())
	}

	answerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/answers", strings.NewReader(`{
		"query":"如何启动服务","project":"app"
	}`))
	answerRequest.Header.Set("X-User-ID", "developer-1")
	answerRequest.Header.Set("X-User-Roles", "developer")
	answerResponse := httptest.NewRecorder()
	handler.ServeHTTP(answerResponse, answerRequest)
	if answerResponse.Code != http.StatusOK || !strings.Contains(answerResponse.Body.String(), "git://app/README.md") {
		t.Fatalf("unexpected answer response: %d %s", answerResponse.Code, answerResponse.Body.String())
	}
	var grounded answer.Response
	if err := json.Unmarshal(answerResponse.Body.Bytes(), &grounded); err != nil {
		t.Fatalf("decode grounded answer: %v", err)
	}
	if grounded.TraceID == "" {
		t.Fatal("grounded answer is missing trace_id")
	}

	traceRequest := httptest.NewRequest(http.MethodGet, "/api/v1/observability/answer-traces/"+grounded.TraceID, nil)
	traceRequest.Header.Set("X-User-ID", "admin-1")
	traceRequest.Header.Set("X-User-Roles", "knowledge_admin")
	traceResponse := httptest.NewRecorder()
	handler.ServeHTTP(traceResponse, traceRequest)
	if traceResponse.Code != http.StatusOK || strings.Contains(traceResponse.Body.String(), `"query"`) || !strings.Contains(traceResponse.Body.String(), `"user_id":"developer-1"`) {
		t.Fatalf("unexpected trace response: %d %s", traceResponse.Code, traceResponse.Body.String())
	}

	metricsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/observability/answer-metrics?project=app", nil)
	metricsRequest.Header.Set("X-User-ID", "admin-1")
	metricsRequest.Header.Set("X-User-Roles", "knowledge_admin")
	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, metricsRequest)
	if metricsResponse.Code != http.StatusOK || !strings.Contains(metricsResponse.Body.String(), `"total":1`) {
		t.Fatalf("unexpected metrics response: %d %s", metricsResponse.Code, metricsResponse.Body.String())
	}

	evaluationRequest := httptest.NewRequest(http.MethodPost, "/api/v1/evaluations/answers", strings.NewReader(`{
		"cases":[{
			"name":"app startup",
			"query":"go run ./cmd/server",
			"project":"app",
			"required_sources":["git://app/README.md"]
		}]
	}`))
	evaluationRequest.Header.Set("X-User-ID", "admin-1")
	evaluationRequest.Header.Set("X-User-Roles", "knowledge_admin")
	evaluationResponse := httptest.NewRecorder()
	handler.ServeHTTP(evaluationResponse, evaluationRequest)
	if evaluationResponse.Code != http.StatusOK || !strings.Contains(evaluationResponse.Body.String(), `"passed":1`) {
		t.Fatalf("unexpected evaluation response: %d %s", evaluationResponse.Code, evaluationResponse.Body.String())
	}
}

func TestAnswerObservabilityRequiresAdminRole(t *testing.T) {
	handler := newTestHandler(t, "")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/observability/answer-metrics", nil)
	request.Header.Set("X-User-ID", "developer-1")
	request.Header.Set("X-User-Roles", "developer")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestTracePruneValidatesRetention(t *testing.T) {
	handler := newTestHandler(t, "")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/observability/answer-traces/prune", strings.NewReader(`{"retention_days":0}`))
	request.Header.Set("X-User-ID", "admin-1")
	request.Header.Set("X-User-Roles", "knowledge_admin")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestEvaluationSuiteAndRunLifecycle(t *testing.T) {
	handler := newTestHandler(t, "")
	adminHeaders := map[string]string{
		"X-User-ID":    "admin-1",
		"X-User-Roles": "knowledge_admin",
	}
	createKnowledge := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/documents", strings.NewReader(`{
		"title":"Startup","content":"Run go run ./cmd/server to start the app.",
		"source_uri":"git://app/README.md","project":"app"
	}`))
	for key, value := range adminHeaders {
		createKnowledge.Header.Set(key, value)
	}
	knowledgeResponse := httptest.NewRecorder()
	handler.ServeHTTP(knowledgeResponse, createKnowledge)
	if knowledgeResponse.Code != http.StatusCreated {
		t.Fatalf("create evaluation knowledge: %d %s", knowledgeResponse.Code, knowledgeResponse.Body.String())
	}

	createSuite := httptest.NewRequest(http.MethodPost, "/api/v1/evaluations/suites", strings.NewReader(`{
		"name":"release-gate","minimum_pass_rate":1,
		"cases":[{
			"name":"startup","query":"go run ./cmd/server","project":"app",
			"required_sources":["git://app/README.md"]
		}]
	}`))
	for key, value := range adminHeaders {
		createSuite.Header.Set(key, value)
	}
	suiteResponse := httptest.NewRecorder()
	handler.ServeHTTP(suiteResponse, createSuite)
	if suiteResponse.Code != http.StatusCreated {
		t.Fatalf("create suite: %d %s", suiteResponse.Code, suiteResponse.Body.String())
	}
	var suite evaluation.Suite
	if err := json.Unmarshal(suiteResponse.Body.Bytes(), &suite); err != nil {
		t.Fatalf("decode suite: %v", err)
	}
	if suite.Version != 1 || suite.DefinitionHash == "" {
		t.Fatalf("unexpected suite: %#v", suite)
	}

	startRun := httptest.NewRequest(http.MethodPost, "/api/v1/evaluations/runs", strings.NewReader(`{"suite_id":"`+suite.ID+`"}`))
	for key, value := range adminHeaders {
		startRun.Header.Set(key, value)
	}
	runResponse := httptest.NewRecorder()
	handler.ServeHTTP(runResponse, startRun)
	if runResponse.Code != http.StatusAccepted {
		t.Fatalf("start run: %d %s", runResponse.Code, runResponse.Body.String())
	}
	var started evaluation.Run
	if err := json.Unmarshal(runResponse.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode started run: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		getRun := httptest.NewRequest(http.MethodGet, "/api/v1/evaluations/runs/"+started.ID, nil)
		for key, value := range adminHeaders {
			getRun.Header.Set(key, value)
		}
		completedResponse := httptest.NewRecorder()
		handler.ServeHTTP(completedResponse, getRun)
		if completedResponse.Code != http.StatusOK {
			t.Fatalf("get run: %d %s", completedResponse.Code, completedResponse.Body.String())
		}
		var completed evaluation.Run
		if err := json.Unmarshal(completedResponse.Body.Bytes(), &completed); err != nil {
			t.Fatalf("decode completed run: %v", err)
		}
		if completed.Status == evaluation.RunCompleted {
			if completed.GateStatus != evaluation.GatePassed || completed.Report.Passed != 1 {
				t.Fatalf("unexpected completed run: %#v", completed)
			}
			retryRequest := httptest.NewRequest(http.MethodPost, "/api/v1/evaluations/runs/"+completed.ID+"/retry", nil)
			for key, value := range adminHeaders {
				retryRequest.Header.Set(key, value)
			}
			retryResponse := httptest.NewRecorder()
			handler.ServeHTTP(retryResponse, retryRequest)
			if retryResponse.Code != http.StatusConflict {
				t.Fatalf("completed quality result must not be retryable: %d %s", retryResponse.Code, retryResponse.Body.String())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("HTTP evaluation run did not complete")
}
