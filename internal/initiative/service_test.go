package initiative

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ekbda/internal/planning"
)

func TestProjectPackageVersionLifecycle(t *testing.T) {
	planningService, session := approvedPlanningService(t)
	service := NewService(NewMemoryStore(), NewLocalProvider(), planningService)
	input := CreateInput{SessionID: session.ID, Name: "order-export", ChangeSummary: "初始立项包"}
	first, err := service.Create(context.Background(), input, "approver-1")
	if err != nil {
		t.Fatalf("create first project package: %v", err)
	}
	if first.Version != 1 || first.DefinitionHash == "" || len(first.Artifacts) != len(RequiredArtifactTypes()) || len(first.Traceability) == 0 || first.Source.PlanningRevision != session.Revision {
		t.Fatalf("unexpected first package: %#v", first)
	}
	for index, expected := range RequiredArtifactTypes() {
		if first.Artifacts[index].Type != expected || len(first.Artifacts[index].Sections) == 0 {
			t.Fatalf("unexpected artifact order: %#v", first.Artifacts)
		}
	}
	first.Artifacts[0].Title = "mutated"
	stored, err := service.Get(context.Background(), first.ID)
	if err != nil || stored.Artifacts[0].Title == "mutated" {
		t.Fatalf("stored package was mutated: %#v err=%v", stored, err)
	}
	input.ChangeSummary = "根据治理评审重新生成"
	second, err := service.Create(context.Background(), input, "approver-1")
	if err != nil || second.Version != 2 || second.ID == first.ID {
		t.Fatalf("create second project package: %#v err=%v", second, err)
	}
	packages, err := service.List(context.Background(), session.Project, input.Name, 10)
	if err != nil || len(packages) != 2 || packages[0].Version != 2 || packages[1].Version != 1 {
		t.Fatalf("unexpected package history: %#v err=%v", packages, err)
	}
}

func TestProjectPackageRequiresApprovedReviewedPlan(t *testing.T) {
	planningService, session := approvedPlanningService(t)
	store := planning.NewMemoryStore()
	session.Status = planning.StatusAwaitingApproval
	if err := store.Create(context.Background(), session, planning.Event{ID: "event-2", SessionID: session.ID, Sequence: 1, Type: "created", ToStatus: session.Status, Actor: "developer-1", CreatedAt: session.CreatedAt}); err != nil {
		t.Fatalf("seed pending session: %v", err)
	}
	planningService = planning.NewService(store, planning.NewLocalProvider(), nil, nil, nil, nil)
	service := NewService(NewMemoryStore(), NewLocalProvider(), planningService)
	_, err := service.Create(context.Background(), CreateInput{SessionID: session.ID, Name: "order-export", ChangeSummary: "must fail"}, "approver-1")
	if !errors.Is(err, ErrPlanningNotApproved) {
		t.Fatalf("pending plan must not create package: %v", err)
	}
}

func TestProjectPackageProviderCannotInventReferences(t *testing.T) {
	planningService, session := approvedPlanningService(t)
	service := NewService(NewMemoryStore(), inventedReferenceProvider{}, planningService)
	_, err := service.Create(context.Background(), CreateInput{SessionID: session.ID, Name: "order-export", ChangeSummary: "invalid output"}, "approver-1")
	if !errors.Is(err, ErrInvalidProviderOutput) {
		t.Fatalf("invented package reference must fail: %v", err)
	}
}

func TestProjectPackageProviderCannotInventTraceabilitySections(t *testing.T) {
	planningService, session := approvedPlanningService(t)
	service := NewService(NewMemoryStore(), invalidTraceProvider{}, planningService)
	_, err := service.Create(context.Background(), CreateInput{SessionID: session.ID, Name: "order-export", ChangeSummary: "invalid trace"}, "approver-1")
	if !errors.Is(err, ErrInvalidProviderOutput) {
		t.Fatalf("invented traceability section must fail: %v", err)
	}
}

type inventedReferenceProvider struct{}

func (inventedReferenceProvider) Name() string { return "invalid" }
func (inventedReferenceProvider) Build(context.Context, planning.Session) (BuildOutput, error) {
	artifacts := make([]Artifact, 0, len(requiredArtifactTypes))
	for _, artifactType := range requiredArtifactTypes {
		artifacts = append(artifacts, Artifact{
			Type: artifactType, Title: artifactType, Summary: "invalid reference",
			Sections:   []Section{{Name: "section", Items: []string{"item"}}},
			References: []Reference{{Kind: ReferencePlanKnowledge, ID: "K999"}},
		})
	}
	return BuildOutput{Artifacts: artifacts}, nil
}

type invalidTraceProvider struct{}

func (invalidTraceProvider) Name() string { return "invalid-trace" }
func (invalidTraceProvider) Build(ctx context.Context, session planning.Session) (BuildOutput, error) {
	output, err := NewLocalProvider().Build(ctx, session)
	if err == nil && len(output.Traceability) > 0 {
		output.Traceability[0].ArchitectureSections = []string{"invented-section"}
	}
	return output, err
}

func TestProjectPackageReviewAndExport(t *testing.T) {
	planningService, session := approvedPlanningService(t)
	service := NewService(NewMemoryStore(), NewLocalProvider(), planningService)
	projectPackage, err := service.Create(context.Background(), CreateInput{SessionID: session.ID, Name: "order-export", ChangeSummary: "reviewable package"}, "approver-1")
	if err != nil {
		t.Fatalf("create project package: %v", err)
	}
	if _, err := service.Review(context.Background(), projectPackage.ID, ReviewInput{ArtifactType: ArtifactPRD, PackageHash: "stale", Decision: "approve", Comment: "wrong hash"}, "approver-2"); !errors.Is(err, ErrPackageHashConflict) {
		t.Fatalf("stale review must fail: %v", err)
	}
	first, err := service.Review(context.Background(), projectPackage.ID, ReviewInput{ArtifactType: ArtifactPRD, PackageHash: projectPackage.DefinitionHash, Decision: "request_changes", Comment: "add measurable acceptance criteria"}, "approver-2")
	if err != nil || first.Sequence != 1 {
		t.Fatalf("create first review: %#v err=%v", first, err)
	}
	second, err := service.Review(context.Background(), projectPackage.ID, ReviewInput{ArtifactType: ArtifactPRD, PackageHash: projectPackage.DefinitionHash, Decision: "approve", Comment: "criteria confirmed"}, "approver-2")
	if err != nil || second.Sequence != 2 {
		t.Fatalf("create second review: %#v err=%v", second, err)
	}
	reviews, err := service.Reviews(context.Background(), projectPackage.ID, ArtifactPRD, 10)
	if err != nil || len(reviews) != 2 || reviews[0].Sequence != 2 {
		t.Fatalf("list reviews: %#v err=%v", reviews, err)
	}
	markdown, err := service.Export(context.Background(), projectPackage.ID, ExportMarkdown)
	if err != nil || !strings.Contains(string(markdown.Data), "REQ-001") || !strings.Contains(string(markdown.Data), "criteria confirmed") {
		t.Fatalf("export Markdown: %v", err)
	}
	docx, err := service.Export(context.Background(), projectPackage.ID, ExportDOCX)
	if err != nil || len(docx.Data) < 2 || string(docx.Data[:2]) != "PK" {
		t.Fatalf("export DOCX: size=%d err=%v", len(docx.Data), err)
	}
}

func approvedPlanningService(t *testing.T) (*planning.Service, planning.Session) {
	t.Helper()
	now := time.Now().UTC()
	reviews := make([]planning.RoleReview, 0, len(planning.RequiredReviewRoles()))
	for _, role := range planning.RequiredReviewRoles() {
		reviews = append(reviews, planning.RoleReview{Role: role, Summary: role + " reviewed", Recommendation: "approve", Findings: []planning.ReviewFinding{}, OpenQuestions: []string{}, KnowledgeReferences: []string{"K1"}, StandardReferences: []string{"S1"}})
	}
	session := planning.Session{
		ID: "approved-session", Project: "order-service", Repository: "order-service", Technology: "go",
		Title: "订单导出", Requirement: "为订单服务增加可审计的批量导出能力。",
		Status: planning.StatusApproved, Revision: 4,
		Plan:    &planning.Plan{Summary: "实现受控订单导出", Steps: []planning.PlanStep{{ID: "P1", Title: "设计", Description: "确认接口与数据流", Deliverables: []string{"设计"}, Verification: []string{"设计评审"}}}, Risks: []planning.Risk{{ID: "R1", Description: "容量风险", Mitigation: "容量测试"}}},
		Context: planning.ContextSnapshot{Hash: "plan-hash", Knowledge: []planning.KnowledgeReference{{ID: "K1", DocumentID: "doc-1", Version: 1}}, Standards: []planning.StandardReference{{ID: "S1", PackageID: "standard-1", Version: 1}}},
		RoleReview: &planning.RoleReviewCycle{
			Provider: "reviewer", Context: planning.ContextSnapshot{Hash: "review-hash", Knowledge: []planning.KnowledgeReference{{ID: "K1", DocumentID: "doc-2", Version: 1}}, Standards: []planning.StandardReference{{ID: "S1", PackageID: "standard-2", Version: 1}}},
			Reviews:      reviews,
			Coordination: planning.Coordination{Summary: "roles coordinated", Consensus: []string{"ship safely"}, DecisionItems: []planning.DecisionItem{{ID: "D1", Topic: "capacity", Description: "capacity decision", Options: []string{"async", "limit"}, SourceRoles: []string{planning.RoleBackendEngineer}, Resolution: "async", ResolvedBy: "approver-1", ResolvedAt: &now}}},
			CompletedAt:  now,
		},
		Provider: "planner", CreatedBy: "developer-1", ReviewedBy: "approver-1", CreatedAt: now, UpdatedAt: now, ReviewedAt: &now,
	}
	store := planning.NewMemoryStore()
	if err := store.Create(context.Background(), session, planning.Event{ID: "event-1", SessionID: session.ID, Sequence: 1, Type: "created", ToStatus: session.Status, Actor: "developer-1", CreatedAt: now}); err != nil {
		t.Fatalf("seed approved planning session: %v", err)
	}
	return planning.NewService(store, planning.NewLocalProvider(), nil, nil, nil, nil), session
}
