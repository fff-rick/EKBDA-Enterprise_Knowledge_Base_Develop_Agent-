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
	"ekbda/internal/development"
	"ekbda/internal/initiative"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

type httpDevelopmentRunner struct{}

func (httpDevelopmentRunner) Enabled() bool                                { return true }
func (httpDevelopmentRunner) RecoveryGracePeriod() time.Duration           { return time.Second }
func (httpDevelopmentRunner) CleanupStale(context.Context, []string) error { return nil }
func (httpDevelopmentRunner) Run(_ context.Context, request development.RunRequest) development.Execution {
	now := time.Now().UTC()
	return development.Execution{
		ID: request.ExecutionID, Status: development.ExecutionPassed, BaselineCommit: request.BaselineCommit,
		PatchHash: request.PatchHash, Isolation: "test", NetworkPolicy: "test", SecretScanPassed: true,
		StandardsReportID: "report-1", StandardsPassed: true, ExecutedBy: request.Actor,
		StartedAt: now, FinishedAt: now, IsolatedCopyRemoved: true,
	}
}

type httpDevelopmentDeliverer struct{}

func (httpDevelopmentDeliverer) Enabled() bool                                { return true }
func (httpDevelopmentDeliverer) RecoveryGracePeriod() time.Duration           { return time.Second }
func (httpDevelopmentDeliverer) CleanupStale(context.Context, []string) error { return nil }
func (httpDevelopmentDeliverer) Deliver(_ context.Context, request development.DeliveryRequest) development.Delivery {
	now := time.Now().UTC()
	return development.Delivery{
		ID: request.DeliveryID, Status: development.DeliveryPassed, Branch: request.Branch,
		Commit: "commit-1", Remote: "origin", BranchPushed: true,
		PullRequestURL: "https://git.example/pull/1", DeliveredBy: request.Actor,
		StartedAt: now, FinishedAt: now, WorkingCopyRemoved: true,
	}
}

func TestDevelopmentProposalHTTPLifecycle(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "order-service")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	for _, arguments := range [][]string{{"init", "--initial-branch=main"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "HTTP Test"}} {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# App\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	for _, arguments := range [][]string{{"add", "."}, {"commit", "--no-gpg-sign", "-m", "initial"}} {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
	}

	initiativeStore := initiative.NewMemoryStore()
	projectPackage, err := initiativeStore.Create(context.Background(), initiative.Package{
		ID: "package-1", Project: "order-service", Repository: "order-service", Name: "order-change",
		DefinitionHash: "package-hash", Source: initiative.SourceSnapshot{PlanningSessionID: "plan-1"},
		Traceability: []initiative.TraceRecord{{CoverageStatus: "covered"}},
	})
	if err != nil {
		t.Fatalf("seed package: %v", err)
	}
	for _, artifactType := range initiative.RequiredArtifactTypes() {
		if _, err := initiativeStore.CreateReview(context.Background(), initiative.ArtifactReview{ID: artifactType, PackageID: projectPackage.ID, ArtifactType: artifactType, PackageHash: projectPackage.DefinitionHash, Decision: "approve"}); err != nil {
			t.Fatalf("seed %s review: %v", artifactType, err)
		}
	}
	initiativeService := initiative.NewService(initiativeStore, nil, nil)
	workspaceService, err := workspace.New(root, standards.NewService(standards.NewMemoryStore()), workspace.NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	accessService := newAccessService(t, access.ModeDisabled)
	developmentService := development.NewServiceWithDelivery(development.NewMemoryStore(), initiativeService, workspaceService, httpDevelopmentRunner{}, httpDevelopmentDeliverer{})
	handler := NewWithDevelopment(nil, nil, nil, nil, nil, workspaceService, accessService, nil, nil, initiativeService, nil, developmentService)

	createBody, _ := json.Marshal(development.CreateInput{ProjectPackageID: projectPackage.ID, AllowedPaths: []string{"README.md"}, AllowedCommands: []string{"git-diff-check"}})
	create := httptest.NewRequest(http.MethodPost, "/api/v1/development/sessions", strings.NewReader(string(createBody)))
	create.Header.Set("X-User-ID", "developer-1")
	create.Header.Set("X-User-Roles", "developer")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", createResponse.Code, createResponse.Body.String())
	}
	var session development.Session
	if err := json.Unmarshal(createResponse.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	patch := "diff --git a/README.md b/README.md\nindex 1111111..2222222 100644\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-# App\n+# App v2\n"
	proposalBody, _ := json.Marshal(development.SubmitInput{Revision: session.Revision, Summary: "update readme", Patch: patch, CommandIDs: []string{"git-diff-check"}})
	proposal := httptest.NewRequest(http.MethodPost, "/api/v1/development/sessions/"+session.ID+"/proposals", strings.NewReader(string(proposalBody)))
	proposal.Header.Set("X-User-ID", "developer-1")
	proposal.Header.Set("X-User-Roles", "developer")
	proposalResponse := httptest.NewRecorder()
	handler.ServeHTTP(proposalResponse, proposal)
	if proposalResponse.Code != http.StatusOK || strings.Contains(proposalResponse.Body.String(), "# App v2") || strings.Contains(proposalResponse.Body.String(), `"patch"`) {
		t.Fatalf("proposal metadata leaked patch or failed: %d %s", proposalResponse.Code, proposalResponse.Body.String())
	}
	if err := json.Unmarshal(proposalResponse.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode proposal response: %v", err)
	}

	preview := httptest.NewRequest(http.MethodGet, "/api/v1/development/sessions/"+session.ID+"/preview", nil)
	preview.Header.Set("X-User-ID", "developer-1")
	preview.Header.Set("X-User-Roles", "developer")
	previewResponse := httptest.NewRecorder()
	handler.ServeHTTP(previewResponse, preview)
	if previewResponse.Code != http.StatusOK || !strings.Contains(previewResponse.Body.String(), "# App v2") {
		t.Fatalf("preview: %d %s", previewResponse.Code, previewResponse.Body.String())
	}

	decisionBody, _ := json.Marshal(development.DecisionInput{Revision: session.Revision, Decision: "approve", Comment: "reviewed"})
	decision := httptest.NewRequest(http.MethodPost, "/api/v1/development/sessions/"+session.ID+"/decision", strings.NewReader(string(decisionBody)))
	decision.Header.Set("X-User-ID", "approver-1")
	decision.Header.Set("X-User-Roles", "project_approver")
	decisionResponse := httptest.NewRecorder()
	handler.ServeHTTP(decisionResponse, decision)
	if decisionResponse.Code != http.StatusOK || !strings.Contains(decisionResponse.Body.String(), `"status":"approved"`) {
		t.Fatalf("decision: %d %s", decisionResponse.Code, decisionResponse.Body.String())
	}
	if err := json.Unmarshal(decisionResponse.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode decision response: %v", err)
	}
	executeBody, _ := json.Marshal(development.ExecuteInput{Revision: session.Revision, PatchHash: session.Proposal.PatchHash, Confirmation: "execute_approved_patch"})
	executeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/development/sessions/"+session.ID+"/execute", strings.NewReader(string(executeBody)))
	executeRequest.Header.Set("X-User-ID", "developer-1")
	executeRequest.Header.Set("X-User-Roles", "developer")
	executeResponse := httptest.NewRecorder()
	handler.ServeHTTP(executeResponse, executeRequest)
	if executeResponse.Code != http.StatusOK || !strings.Contains(executeResponse.Body.String(), `"status":"verified"`) || !strings.Contains(executeResponse.Body.String(), `"standards_report_id":"report-1"`) {
		t.Fatalf("execute: %d %s", executeResponse.Code, executeResponse.Body.String())
	}
	if err := json.Unmarshal(executeResponse.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode execute response: %v", err)
	}
	deliveryBody, _ := json.Marshal(development.DeliverInput{Revision: session.Revision, PatchHash: session.Proposal.PatchHash, Confirmation: "deliver_verified_change"})
	deliveryRequest := httptest.NewRequest(http.MethodPost, "/api/v1/development/sessions/"+session.ID+"/deliver", strings.NewReader(string(deliveryBody)))
	deliveryRequest.Header.Set("X-User-ID", "approver-1")
	deliveryRequest.Header.Set("X-User-Roles", "project_approver")
	deliveryResponse := httptest.NewRecorder()
	handler.ServeHTTP(deliveryResponse, deliveryRequest)
	if deliveryResponse.Code != http.StatusOK || !strings.Contains(deliveryResponse.Body.String(), `"status":"delivered"`) || !strings.Contains(deliveryResponse.Body.String(), `"pull_request_url":"https://git.example/pull/1"`) {
		t.Fatalf("deliver: %d %s", deliveryResponse.Code, deliveryResponse.Body.String())
	}
}
