package development

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ekbda/internal/initiative"
	"ekbda/internal/workspace"
)

type fakeInitiative struct {
	projectPackage initiative.Package
	reviews        []initiative.ArtifactReview
}

func (f fakeInitiative) Get(context.Context, string) (initiative.Package, error) {
	return f.projectPackage, nil
}

func (f fakeInitiative) Reviews(context.Context, string, string, int) ([]initiative.ArtifactReview, error) {
	return append([]initiative.ArtifactReview(nil), f.reviews...), nil
}

type fakeWorkspace struct{ state workspace.RepositoryState }

func (f *fakeWorkspace) Inspect(context.Context, string) (workspace.RepositoryState, error) {
	return f.state, nil
}

type fakeRunner struct {
	cleanupCalled bool
}

type fakeDeliverer struct {
	cleanupCalled bool
}

func (*fakeDeliverer) Enabled() bool                      { return true }
func (*fakeDeliverer) RecoveryGracePeriod() time.Duration { return time.Second }
func (d *fakeDeliverer) CleanupStale(context.Context, []string) error {
	d.cleanupCalled = true
	return nil
}
func (*fakeDeliverer) Deliver(_ context.Context, request DeliveryRequest) Delivery {
	now := time.Now().UTC()
	return Delivery{
		ID: request.DeliveryID, Status: DeliveryPassed, Branch: request.Branch, Commit: "commit-1",
		Remote: "origin", BranchPushed: true, PullRequestURL: "https://git.example/pull/1",
		DeliveredBy: request.Actor, StartedAt: now, FinishedAt: now, WorkingCopyRemoved: true,
	}
}

func (*fakeRunner) Enabled() bool                      { return true }
func (*fakeRunner) RecoveryGracePeriod() time.Duration { return time.Second }
func (r *fakeRunner) CleanupStale(context.Context, []string) error {
	r.cleanupCalled = true
	return nil
}
func (*fakeRunner) Run(_ context.Context, request RunRequest) Execution {
	return Execution{
		ID: request.ExecutionID, Status: ExecutionPassed, BaselineCommit: request.BaselineCommit,
		PatchHash: request.PatchHash, Isolation: "fake", NetworkPolicy: "test", SecretScanPassed: true,
		StandardsReportID: "report-1", StandardsPassed: true, ExecutedBy: request.Actor,
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(), IsolatedCopyRemoved: true,
	}
}

func approvedInitiative() fakeInitiative {
	projectPackage := initiative.Package{
		ID: "package-1", Project: "order", Repository: "order-service", Name: "order-v1", Version: 1,
		DefinitionHash: "package-hash", Source: initiative.SourceSnapshot{PlanningSessionID: "plan-1"},
		Traceability: []initiative.TraceRecord{{CoverageStatus: "covered"}},
	}
	reviews := make([]initiative.ArtifactReview, 0)
	for _, artifactType := range initiative.RequiredArtifactTypes() {
		reviews = append(reviews, initiative.ArtifactReview{ArtifactType: artifactType, PackageHash: projectPackage.DefinitionHash, Decision: "approve", Sequence: 1})
	}
	return fakeInitiative{projectPackage: projectPackage, reviews: reviews}
}

func TestServiceProposalApprovalWorkflow(t *testing.T) {
	ctx := context.Background()
	workspaceReader := &fakeWorkspace{state: workspace.RepositoryState{Repository: "order-service", HeadCommit: "0123456789012345678901234567890123456789", Branch: "main"}}
	runner := &fakeRunner{}
	deliverer := &fakeDeliverer{}
	store := NewMemoryStore()
	initiativeReader := approvedInitiative()
	service := NewServiceWithDelivery(store, initiativeReader, workspaceReader, runner, deliverer)
	session, err := service.Create(ctx, CreateInput{ProjectPackageID: "package-1", AllowedPaths: []string{"internal"}, AllowedCommands: []string{"go-test-all", "git-diff-check"}}, "developer-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if session.Status != StatusDraft || session.BaselineCommit != workspaceReader.state.HeadCommit || session.PlannedBranch == "" {
		t.Fatalf("unexpected session: %#v", session)
	}
	session, err = service.Submit(ctx, session.ID, SubmitInput{Revision: session.Revision, Summary: "更新订单校验", Patch: validPatch, CommandIDs: []string{"git-diff-check", "go-test-all"}}, "developer-1")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if session.Status != StatusAwaitingApproval || session.Proposal == nil || session.Proposal.PatchHash == "" {
		t.Fatalf("unexpected proposal: %#v", session)
	}
	metadata, err := json.Marshal(session)
	if err != nil || strings.Contains(string(metadata), validPatch) || strings.Contains(string(metadata), `"patch"`) {
		t.Fatalf("ordinary session JSON leaked patch: %s, %v", metadata, err)
	}
	preview, err := service.Preview(ctx, session.ID)
	if err != nil || preview.Patch != validPatch {
		t.Fatalf("preview: %#v, %v", preview, err)
	}
	if _, err := service.Decide(ctx, session.ID, DecisionInput{Revision: session.Revision, Decision: "approve", Comment: "通过"}, "developer-1"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self approval got %v", err)
	}
	session, err = service.Decide(ctx, session.ID, DecisionInput{Revision: session.Revision, Decision: "approve", Comment: "验证范围清晰"}, "approver-1")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if session.Status != StatusApproved || session.ReviewedBy != "approver-1" {
		t.Fatalf("unexpected approved session: %#v", session)
	}
	disabledService := NewService(store, initiativeReader, workspaceReader)
	if _, err := disabledService.Execute(ctx, session.ID, ExecuteInput{Revision: session.Revision, PatchHash: session.Proposal.PatchHash, Confirmation: "execute_approved_patch"}, "developer-1"); !errors.Is(err, ErrExecutionDisabled) {
		t.Fatalf("disabled execution got %v", err)
	}
	if _, err := service.Execute(ctx, session.ID, ExecuteInput{Revision: session.Revision, PatchHash: "wrong-hash", Confirmation: "execute_approved_patch"}, "developer-1"); !errors.Is(err, ErrExecutionConflict) {
		t.Fatalf("mismatched execution confirmation got %v", err)
	}
	session, err = service.Execute(ctx, session.ID, ExecuteInput{Revision: session.Revision, PatchHash: session.Proposal.PatchHash, Confirmation: "execute_approved_patch"}, "developer-1")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if session.Status != StatusVerified || session.Execution == nil || session.Execution.Status != ExecutionPassed {
		t.Fatalf("unexpected verified session: %#v", session)
	}
	if _, err := service.Deliver(ctx, session.ID, DeliverInput{Revision: session.Revision, PatchHash: session.Proposal.PatchHash, Confirmation: "deliver_verified_change"}, "developer-1"); !errors.Is(err, ErrSelfDelivery) {
		t.Fatalf("self delivery got %v", err)
	}
	if _, err := disabledService.Deliver(ctx, session.ID, DeliverInput{Revision: session.Revision, PatchHash: session.Proposal.PatchHash, Confirmation: "deliver_verified_change"}, "approver-1"); !errors.Is(err, ErrDeliveryDisabled) {
		t.Fatalf("disabled delivery got %v", err)
	}
	session, err = service.Deliver(ctx, session.ID, DeliverInput{Revision: session.Revision, PatchHash: session.Proposal.PatchHash, Confirmation: "deliver_verified_change"}, "approver-1")
	if err != nil || session.Status != StatusDelivered || session.Delivery == nil || session.Delivery.PullRequestURL == "" {
		t.Fatalf("deliver: %#v, %v", session, err)
	}
	events, err := service.Events(ctx, session.ID)
	if err != nil || len(events) != 7 || events[2].Type != EventApproved || events[4].Type != EventExecutionPassed || events[6].Type != EventDeliveryPassed {
		t.Fatalf("unexpected events: %#v, %v", events, err)
	}
}

func TestServiceBlocksUnapprovedPackageAndWorkspaceDrift(t *testing.T) {
	ctx := context.Background()
	initiativeReader := approvedInitiative()
	initiativeReader.reviews = initiativeReader.reviews[:len(initiativeReader.reviews)-1]
	workspaceReader := &fakeWorkspace{state: workspace.RepositoryState{HeadCommit: "0123456789012345678901234567890123456789"}}
	service := NewService(NewMemoryStore(), initiativeReader, workspaceReader)
	if _, err := service.Create(ctx, CreateInput{ProjectPackageID: "package-1", AllowedPaths: []string{"internal"}, AllowedCommands: []string{"go-test-all"}}, "developer-1"); !errors.Is(err, ErrPackageNotApproved) {
		t.Fatalf("unapproved package got %v", err)
	}
	service = NewService(NewMemoryStore(), approvedInitiative(), workspaceReader)
	session, err := service.Create(ctx, CreateInput{ProjectPackageID: "package-1", AllowedPaths: []string{"internal"}, AllowedCommands: []string{"go-test-all"}}, "developer-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	workspaceReader.state.HeadCommit = "1123456789012345678901234567890123456789"
	_, err = service.Submit(ctx, session.ID, SubmitInput{Revision: session.Revision, Summary: "更新", Patch: validPatch, CommandIDs: []string{"go-test-all"}}, "developer-1")
	if !errors.Is(err, ErrBaselineChanged) {
		t.Fatalf("baseline drift got %v", err)
	}
}

func TestServiceRecoversInterruptedExecution(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Now().UTC().Add(-time.Minute)
	session := Session{
		ID: "session-interrupted", Project: "order", Repository: "order-service", Status: StatusExecuting,
		Revision: 4, CreatedAt: now, UpdatedAt: now,
		Execution: &Execution{ID: "execution-interrupted", Status: ExecutionRunning, StartedAt: now},
	}
	if err := store.Create(ctx, session, Event{ID: "event-1", SessionID: session.ID, Sequence: 1, Type: EventCreated, ToStatus: StatusDraft, Actor: "developer", CreatedAt: now}); err != nil {
		t.Fatalf("seed interrupted session: %v", err)
	}
	recent := session
	recent.ID = "session-recent"
	recent.Revision = 2
	recent.UpdatedAt = time.Now().UTC()
	recent.Execution = &Execution{ID: "execution-recent", Status: ExecutionRunning, StartedAt: recent.UpdatedAt}
	if err := store.Create(ctx, recent, Event{ID: "event-recent", SessionID: recent.ID, Sequence: 1, Type: EventCreated, ToStatus: StatusDraft, Actor: "developer", CreatedAt: recent.UpdatedAt}); err != nil {
		t.Fatalf("seed recent execution: %v", err)
	}
	runner := &fakeRunner{}
	service := NewServiceWithRunner(store, approvedInitiative(), &fakeWorkspace{}, runner)
	if err := service.Recover(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	loaded, err := service.Get(ctx, session.ID)
	if err != nil || loaded.Status != StatusExecutionFailed || loaded.Execution == nil || loaded.Execution.ErrorCode != "execution_interrupted" || !runner.cleanupCalled {
		t.Fatalf("unexpected recovered session: %#v, %v", loaded, err)
	}
	events, err := service.Events(ctx, session.ID)
	if err != nil || len(events) != 2 || events[1].Type != EventExecutionRecovered {
		t.Fatalf("unexpected recovery events: %#v, %v", events, err)
	}
	recentLoaded, err := service.Get(ctx, recent.ID)
	if err != nil || recentLoaded.Status != StatusExecuting {
		t.Fatalf("recent execution must not be recovered: %#v, %v", recentLoaded, err)
	}
}

func TestServiceRecoversInterruptedDelivery(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Now().UTC().Add(-time.Minute)
	session := Session{
		ID: "session-delivery-interrupted", Project: "order", Repository: "order-service", Status: StatusDelivering,
		Revision: 6, PlannedBranch: "codex/order/session", CreatedAt: now, UpdatedAt: now,
		Delivery: &Delivery{ID: "delivery-interrupted", Status: DeliveryRunning, StartedAt: now},
	}
	if err := store.Create(ctx, session, Event{ID: "event-1", SessionID: session.ID, Sequence: 1, Type: EventCreated, ToStatus: StatusDraft, Actor: "developer", CreatedAt: now}); err != nil {
		t.Fatalf("seed interrupted delivery: %v", err)
	}
	deliverer := &fakeDeliverer{}
	service := NewServiceWithDelivery(store, approvedInitiative(), &fakeWorkspace{}, disabledRunner{}, deliverer)
	if err := service.Recover(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	loaded, err := service.Get(ctx, session.ID)
	if err != nil || loaded.Status != StatusDeliveryFailed || loaded.Delivery == nil || loaded.Delivery.ErrorCode != "delivery_interrupted" || !deliverer.cleanupCalled {
		t.Fatalf("unexpected recovered delivery: %#v, %v", loaded, err)
	}
	events, err := service.Events(ctx, session.ID)
	if err != nil || len(events) != 2 || events[1].Type != EventDeliveryRecovered {
		t.Fatalf("unexpected recovery events: %#v, %v", events, err)
	}
}
