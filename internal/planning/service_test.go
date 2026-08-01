package planning

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ekbda/internal/embedding"
	"ekbda/internal/knowledge"
	"ekbda/internal/repositorysync"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

func TestPlanningSessionClarificationApprovalLifecycle(t *testing.T) {
	service, restrictedDocumentID := newPlanningService(t, NewLocalProvider())
	ctx := context.Background()
	session, err := service.Create(ctx, CreateInput{
		Project: "order-service", Repository: "order-service", Technology: "go",
		Title: "订单导出", Requirement: "为订单服务增加可审计的批量导出能力，并保持现有查询接口兼容。",
	}, "developer-1", []string{"developer"})
	if err != nil {
		t.Fatalf("create planning session: %v", err)
	}
	if session.Status != StatusAwaitingClarification || session.Revision != 1 || len(session.Questions) != 3 || session.Context.Hash == "" || session.Context.Repository.HeadCommit == "" {
		t.Fatalf("unexpected created session: %#v", session)
	}
	for _, reference := range session.Context.Knowledge {
		if reference.Snippet != "" || reference.Title != "" || reference.Citation != nil {
			t.Fatalf("persisted context leaked knowledge content: %#v", reference)
		}
		if reference.DocumentID == restrictedDocumentID {
			t.Fatalf("unauthorized restricted knowledge entered planning context: %#v", reference)
		}
	}
	if _, err := service.SubmitClarifications(ctx, session.ID, ClarificationInput{Revision: 99}, "developer-1", []string{"developer"}, false); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision must fail: %v", err)
	}
	answers := make([]ClarificationAnswer, 0, len(session.Questions))
	for _, question := range session.Questions {
		answers = append(answers, ClarificationAnswer{QuestionID: question.ID, Answer: "已由产品和研发共同确认该项约束。"})
	}
	if _, err := service.SubmitClarifications(ctx, session.ID, ClarificationInput{Revision: 1, Answers: answers}, "other-user", []string{"developer"}, false); !errors.Is(err, ErrForbiddenParticipant) {
		t.Fatalf("non-creator clarification must fail: %v", err)
	}
	planned, err := service.SubmitClarifications(ctx, session.ID, ClarificationInput{Revision: 1, Answers: answers}, "developer-1", []string{"developer"}, false)
	if err != nil {
		t.Fatalf("submit clarifications: %v", err)
	}
	if planned.Status != StatusAwaitingRoleReview || planned.Revision != 2 || planned.Plan == nil || len(planned.Plan.Steps) != 4 {
		t.Fatalf("unexpected planned session: %#v", planned)
	}
	if _, err := service.Decide(ctx, session.ID, DecisionInput{Revision: 2, Decision: "approve"}, "approver-1"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("approval before role review must fail: %v", err)
	}
	if _, err := service.SubmitRoleReviews(ctx, session.ID, 2, "other-user", []string{"developer"}, false); !errors.Is(err, ErrForbiddenParticipant) {
		t.Fatalf("unrelated user role review must fail: %v", err)
	}
	if _, err := service.SubmitRoleReviews(ctx, session.ID, 99, "developer-1", []string{"developer"}, false); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale role review revision must fail: %v", err)
	}
	reviewed, err := service.SubmitRoleReviews(ctx, session.ID, 2, "developer-1", []string{"developer"}, false)
	if err != nil {
		t.Fatalf("complete role reviews: %v", err)
	}
	if reviewed.Status != StatusAwaitingApproval || reviewed.Revision != 3 || reviewed.RoleReview == nil || len(reviewed.RoleReview.Reviews) != len(RequiredReviewRoles()) || len(reviewed.RoleReview.Coordination.DecisionItems) != 0 {
		t.Fatalf("unexpected role review result: %#v", reviewed)
	}
	for _, reference := range reviewed.RoleReview.Context.Knowledge {
		if reference.Snippet != "" || reference.Title != "" || reference.Citation != nil {
			t.Fatalf("persisted role review context leaked knowledge content: %#v", reference)
		}
	}
	if _, err := service.Decide(ctx, session.ID, DecisionInput{Revision: 3, Decision: "approve"}, "developer-1"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("creator self approval must fail: %v", err)
	}
	approved, err := service.Decide(ctx, session.ID, DecisionInput{Revision: 3, Decision: "approve", Reason: "设计和验收标准已评审"}, "approver-1")
	if err != nil {
		t.Fatalf("approve planning session: %v", err)
	}
	if approved.Status != StatusApproved || approved.Revision != 4 || approved.ReviewedBy != "approver-1" || approved.ReviewedAt == nil {
		t.Fatalf("unexpected approved session: %#v", approved)
	}
	events, err := service.Events(ctx, session.ID)
	if err != nil || len(events) != 4 || events[0].Type != "created" || events[1].Type != "clarifications_submitted" || events[2].Type != "role_reviews_completed" || events[3].Type != "approved" {
		t.Fatalf("unexpected planning events: %#v err=%v", events, err)
	}
}

func TestRoleReviewConflictRequiresIndependentResolution(t *testing.T) {
	provider := &blockingReviewProvider{LocalProvider: NewLocalProvider()}
	service, _ := newPlanningService(t, provider)
	ctx := context.Background()
	session, err := service.Create(ctx, CreateInput{
		Project: "order-service", Repository: "order-service", Technology: "go",
		Title: "订单导出", Requirement: "为订单服务增加批量导出能力并提供自动化测试。",
		AcceptanceCriteria: []string{"导出结果可下载"}, Constraints: []string{"保持接口兼容"}, OutOfScope: []string{"不改造前端"},
	}, "developer-1", []string{"developer"})
	if err != nil {
		t.Fatalf("create planning session: %v", err)
	}
	if session.Status != StatusAwaitingRoleReview || session.Revision != 1 {
		t.Fatalf("unexpected initial status: %#v", session)
	}
	reviewed, err := service.SubmitRoleReviews(ctx, session.ID, 1, "developer-1", []string{"developer"}, false)
	if err != nil {
		t.Fatalf("complete blocking reviews: %v", err)
	}
	if reviewed.Status != StatusAwaitingResolution || reviewed.Revision != 2 || len(reviewed.RoleReview.Coordination.DecisionItems) != 1 {
		t.Fatalf("unexpected conflict result: %#v", reviewed)
	}
	resolution := ResolutionInput{Revision: 2, Resolutions: []Resolution{{DecisionID: "D1", Resolution: "补充容量测试后继续"}}}
	if _, err := service.ResolveReviewDecisions(ctx, session.ID, resolution, "developer-1"); !errors.Is(err, ErrSelfResolution) {
		t.Fatalf("creator resolution must fail: %v", err)
	}
	if _, err := service.ResolveReviewDecisions(ctx, session.ID, ResolutionInput{Revision: 2}, "approver-1"); !errors.Is(err, ErrIncompleteResolutions) {
		t.Fatalf("incomplete resolution must fail: %v", err)
	}
	resolved, err := service.ResolveReviewDecisions(ctx, session.ID, resolution, "approver-1")
	if err != nil {
		t.Fatalf("resolve review decision: %v", err)
	}
	decision := resolved.RoleReview.Coordination.DecisionItems[0]
	if resolved.Status != StatusAwaitingApproval || resolved.Revision != 3 || decision.ResolvedBy != "approver-1" || decision.ResolvedAt == nil {
		t.Fatalf("unexpected resolution result: %#v", resolved)
	}
	events, err := service.Events(ctx, session.ID)
	if err != nil || len(events) != 3 || events[1].Type != "role_reviews_completed" || events[2].Type != "review_decisions_resolved" {
		t.Fatalf("unexpected review resolution events: %#v err=%v", events, err)
	}
}

type blockingReviewProvider struct{ *LocalProvider }

func (*blockingReviewProvider) Coordinate(context.Context, Session, []RoleReview) (Coordination, error) {
	return Coordination{
		Summary:   "后端与运维评审发现需要人工决策的容量风险。",
		Consensus: []string{"需要保留自动化测试和回滚方案。"},
		DecisionItems: []DecisionItem{{
			ID: "D1", Topic: "导出容量", Description: "十万条订单导出可能影响在线流量。",
			Options: []string{"增加异步任务和限流", "降低单次导出上限"}, SourceRoles: []string{RoleBackendEngineer, RoleOperationsEngineer},
		}},
	}, nil
}

func TestPlanningProviderCannotInventReferences(t *testing.T) {
	service, _ := newPlanningService(t, invalidReferenceProvider{})
	_, err := service.Create(context.Background(), CreateInput{
		Project: "order-service", Repository: "order-service", Technology: "go",
		Title: "订单导出", Requirement: "为订单服务增加批量导出能力并提供自动化测试。",
		AcceptanceCriteria: []string{"导出结果可下载"}, Constraints: []string{"保持接口兼容"}, OutOfScope: []string{"不改造前端"},
	}, "developer-1", []string{"developer"})
	if !errors.Is(err, ErrInvalidProviderOutput) {
		t.Fatalf("invented reference must be rejected: %v", err)
	}
}

type invalidReferenceProvider struct{}

func (invalidReferenceProvider) Name() string { return "invalid" }
func (invalidReferenceProvider) Clarify(context.Context, Session) ([]Question, error) {
	return nil, nil
}
func (invalidReferenceProvider) BuildPlan(context.Context, Session) (Plan, error) {
	return Plan{Summary: "invalid", Steps: []PlanStep{{
		ID: "P1", Title: "invalid", Description: "invalid reference",
		Deliverables: []string{"plan"}, Verification: []string{"review"}, KnowledgeReferences: []string{"K999"},
	}}}, nil
}

func newPlanningService(t *testing.T, provider Provider) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "order-service")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	for _, arguments := range [][]string{{"init", "--initial-branch=main"}, {"config", "user.email", "planning@example.com"}, {"config", "user.name", "Planning Test"}} {
		runPlanningGit(t, repositoryPath, arguments...)
	}
	if err := os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("# Order Service\n"), 0o600); err != nil {
		t.Fatalf("write repository file: %v", err)
	}
	runPlanningGit(t, repositoryPath, "add", ".")
	runPlanningGit(t, repositoryPath, "commit", "--no-gpg-sign", "-m", "initial")

	knowledgeRepository := knowledge.NewMemoryRepository()
	knowledgeService := knowledge.NewService(knowledgeRepository, embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title: "订单导出流程", Content: "订单服务现有查询接口需要保持兼容，导出操作必须记录审计信息。",
		SourceURI: "wiki://order/export", Project: "order-service", Classification: knowledge.ClassificationInternal,
	})
	if err != nil {
		t.Fatalf("create internal knowledge: %v", err)
	}
	restricted, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title: "订单密钥", Content: "订单导出使用受限密钥。", SourceURI: "wiki://order/secret",
		Project: "order-service", Classification: knowledge.ClassificationRestricted, AllowedRoles: []string{"security"},
	})
	if err != nil {
		t.Fatalf("create restricted knowledge: %v", err)
	}
	standardsService := standards.NewService(standards.NewMemoryStore())
	_, err = standardsService.CreatePackage(context.Background(), standards.CreatePackageInput{
		Name: "go-service", Scope: standards.ScopeTechnology, Selector: "go", Owner: "platform",
		Rules: []standards.Rule{{ID: "tests", Title: "必须提供测试", Description: "新增逻辑必须有自动化测试", Category: standards.CategoryTesting, Level: standards.LevelGuidance}},
	}, "admin-1")
	if err != nil {
		t.Fatalf("create standards package: %v", err)
	}
	workspaceService, err := workspace.New(root, standardsService, workspace.NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace service: %v", err)
	}
	repositorySyncService := repositorysync.New(workspaceService, knowledgeService, repositorysync.NewMemoryStore())
	return NewService(NewMemoryStore(), provider, knowledgeService, standardsService, workspaceService, repositorySyncService), restricted.ID
}

func runPlanningGit(t *testing.T, repository string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, strings.TrimSpace(string(output)))
	}
}
