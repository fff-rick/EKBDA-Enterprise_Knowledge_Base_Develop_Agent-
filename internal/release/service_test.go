package release

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fixedDevelopment struct{ session DevelopmentSession }

func (f fixedDevelopment) Get(context.Context, string) (DevelopmentSession, error) {
	return f.session, nil
}

type fixedConnector struct {
	trigger, rollback ProviderRun
	triggerErr        error
}

func (fixedConnector) Enabled() bool { return true }
func (f fixedConnector) Trigger(context.Context, TriggerRequest) (ProviderRun, error) {
	return f.trigger, f.triggerErr
}
func (f fixedConnector) Rollback(context.Context, RollbackRequest) (ProviderRun, error) {
	return f.rollback, nil
}

func newReleaseService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(NewMemoryStore(), fixedDevelopment{DevelopmentSession{ID: "dev-1", Project: "order-service", Repository: "services/order", Status: "delivered", DeliveryStatus: "passed", Commit: "0123456789abcdef0123456789abcdef01234567", PullRequestURL: "https://git.example.test/acme/order/pull/7"}}, fixedConnector{trigger: ProviderRun{ID: "run-1", URL: "https://ci.example.test/runs/1"}, rollback: ProviderRun{ID: "rollback-1", URL: "https://ci.example.test/runs/2"}}, []string{"deploy"}, []string{"staging", "production"})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC) }
	return service
}

func createApprovedTriggered(t *testing.T, service *Service) Request {
	t.Helper()
	ctx := context.Background()
	value, err := service.Create(ctx, CreateInput{DevelopmentSessionID: "dev-1", Environment: "staging", Pipeline: "deploy", ChangeTicket: "CHG-123", ManifestSHA256: hash64("a"), ConfigurationSHA256: hash64("b"), RollbackPlan: "restore the previously signed artifact"}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err = service.ReconcileCodePlatform(ctx, mergedSourceEvent(value), []byte(`{"event":"merged"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Decide(ctx, value.ID, DecisionInput{Revision: value.Revision, Decision: "approve", Comment: "risk accepted"}, "approver-1"); err != nil {
		t.Fatal(err)
	}
	value, err = service.Get(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	value, err = service.Trigger(ctx, value.ID, TriggerInput{Revision: value.Revision, Confirmation: value.TriggerConfirmation}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestReleaseLifecycleRequiresTrustedEvidenceAndSupportsRollback(t *testing.T) {
	service := newReleaseService(t)
	ctx := context.Background()
	value := createApprovedTriggered(t, service)
	event := successfulEvent(value)
	body := []byte(`{"event":"success"}`)
	completed, applied, err := service.Reconcile(ctx, event, body)
	if err != nil {
		t.Fatal(err)
	}
	if !applied || completed.Status != StatusSucceeded || completed.Artifact == nil {
		t.Fatalf("unexpected release result: %#v applied=%v", completed, applied)
	}
	duplicate, applied, err := service.Reconcile(ctx, event, body)
	if err != nil {
		t.Fatal(err)
	}
	if applied || duplicate.Revision != completed.Revision {
		t.Fatalf("duplicate event was not idempotent: %#v", duplicate)
	}
	rollback, err := service.RequestRollback(ctx, completed.ID, RollbackInput{Revision: completed.Revision, Reason: "error rate breached the release SLO"}, "engineer-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.DecideRollback(ctx, rollback.ID, DecisionInput{Revision: rollback.Revision, Decision: "approve", Comment: "rollback approved"}, "engineer-2"); !errors.Is(err, ErrSelfRollbackApproval) {
		t.Fatalf("expected self rollback approval rejection, got %v", err)
	}
	rollback, err = service.DecideRollback(ctx, rollback.ID, DecisionInput{Revision: rollback.Revision, Decision: "approve", Comment: "rollback approved"}, "approver-2")
	if err != nil {
		t.Fatal(err)
	}
	rollback, err = service.TriggerRollback(ctx, rollback.ID, TriggerInput{Revision: rollback.Revision, Confirmation: rollback.TriggerConfirmation}, "engineer-2")
	if err != nil {
		t.Fatal(err)
	}
	rollbackEvent := ProviderEvent{EventID: "event-rollback-1", ReleaseID: rollback.ID, RunID: rollback.RollbackRunID, Phase: ProviderPhaseRollback, Status: ProviderStatusSucceeded, Checks: []CheckEvidence{check("health"), check("smoke")}}
	rolledBack, _, err := service.Reconcile(ctx, rollbackEvent, []byte(`{"event":"rollback"}`))
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Status != StatusRolledBack {
		t.Fatalf("expected rolled back, got %s", rolledBack.Status)
	}
}

func TestReleaseSuccessWithoutSupplyChainEvidenceBecomesFailed(t *testing.T) {
	service := newReleaseService(t)
	value := createApprovedTriggered(t, service)
	event := successfulEvent(value)
	event.Artifact.SignatureVerified = false
	failed, applied, err := service.Reconcile(context.Background(), event, []byte(`{"event":"unsigned"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !applied || failed.Status != StatusFailed || failed.ErrorCode != "release_evidence_failed" {
		t.Fatalf("untrusted success was accepted: %#v", failed)
	}
}

func TestReleaseApprovalAndProviderIdentitySeparation(t *testing.T) {
	service := newReleaseService(t)
	ctx := context.Background()
	value, err := service.Create(ctx, CreateInput{DevelopmentSessionID: "dev-1", Environment: "staging", Pipeline: "deploy", ChangeTicket: "CHG-124", ManifestSHA256: hash64("c"), ConfigurationSHA256: hash64("d"), RollbackPlan: "restore the previous deployment safely"}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err = service.ReconcileCodePlatform(ctx, mergedSourceEvent(value), []byte(`{"event":"merged-two"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Decide(ctx, value.ID, DecisionInput{Revision: value.Revision, Decision: "approve"}, "engineer-1"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("expected self approval rejection, got %v", err)
	}
	value, err = service.Decide(ctx, value.ID, DecisionInput{Revision: value.Revision, Decision: "approve"}, "approver-1")
	if err != nil {
		t.Fatal(err)
	}
	value, err = service.Trigger(ctx, value.ID, TriggerInput{Revision: value.Revision, Confirmation: value.TriggerConfirmation}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	event := successfulEvent(value)
	if _, _, err = service.Reconcile(ctx, event, []byte(`{"one":1}`)); err != nil {
		t.Fatal(err)
	}
	event.ReleaseID = "another-release"
	if _, _, err = service.Reconcile(ctx, event, []byte(`{"two":2}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected unknown release rejection, got %v", err)
	}
}

func TestProductionReleaseRequiresSuccessfulArtifactPromotion(t *testing.T) {
	service := newReleaseService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, CreateInput{DevelopmentSessionID: "dev-1", Environment: "production", Pipeline: "deploy", ChangeTicket: "CHG-300", ManifestSHA256: hash64("a"), ConfigurationSHA256: hash64("b"), RollbackPlan: "restore the previous signed artifact"}, "engineer-1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("production without promotion accepted: %v", err)
	}
	staging := createApprovedTriggered(t, service)
	event := successfulEvent(staging)
	staging, _, err := service.Reconcile(ctx, event, []byte(`{"event":"staging-success"}`))
	if err != nil {
		t.Fatal(err)
	}
	production, err := service.Create(ctx, CreateInput{DevelopmentSessionID: "dev-1", Environment: "production", Pipeline: "deploy", ChangeTicket: "CHG-301", ManifestSHA256: hash64("a"), ConfigurationSHA256: hash64("b"), RollbackPlan: "restore the previous signed artifact", PromoteFromReleaseID: staging.ID}, "engineer-2")
	if err != nil {
		t.Fatal(err)
	}
	if production.PromotedArtifactDigest != staging.Artifact.Digest {
		t.Fatalf("artifact digest was not pinned: %#v", production)
	}
	production, _, err = service.ReconcileCodePlatform(ctx, mergedSourceEvent(production), []byte(`{"event":"production-merged"}`))
	if err != nil {
		t.Fatal(err)
	}
	production, err = service.Decide(ctx, production.ID, DecisionInput{Revision: production.Revision, Decision: "approve"}, "approver-3")
	if err != nil {
		t.Fatal(err)
	}
	production, err = service.Trigger(ctx, production.ID, TriggerInput{Revision: production.Revision, Confirmation: production.TriggerConfirmation}, "engineer-2")
	if err != nil {
		t.Fatal(err)
	}
	wrongArtifact := successfulEvent(production)
	wrongArtifact.EventID = "production-success-1"
	wrongArtifact.Artifact.Digest = "sha256:" + hash64("9")
	production, _, err = service.Reconcile(ctx, wrongArtifact, []byte(`{"event":"wrong-artifact"}`))
	if err != nil {
		t.Fatal(err)
	}
	if production.Status != StatusFailed || production.ErrorCode != "release_evidence_failed" {
		t.Fatalf("production accepted a different artifact: %#v", production)
	}
}

func TestCodePlatformGateDoesNotTrustIncompleteMergeEvent(t *testing.T) {
	service := newReleaseService(t)
	ctx := context.Background()
	value, err := service.Create(ctx, CreateInput{DevelopmentSessionID: "dev-1", Environment: "staging", Pipeline: "deploy", ChangeTicket: "CHG-400", ManifestSHA256: hash64("a"), ConfigurationSHA256: hash64("b"), RollbackPlan: "restore the previous signed artifact"}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	event := mergedSourceEvent(value)
	event.Approvals = 1
	event.RequiredApprovals = 2
	value, applied, err := service.ReconcileCodePlatform(ctx, event, []byte(`{"event":"insufficient-approval"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !applied || value.Status != StatusAwaitingSourceVerification || value.TriggerConfirmation != "" {
		t.Fatalf("incomplete source promotion was trusted: %#v", value)
	}
	if _, err = service.Decide(ctx, value.ID, DecisionInput{Revision: value.Revision, Decision: "approve"}, "approver-1"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("release was approvable before source verification: %v", err)
	}
}

func mergedSourceEvent(value Request) CodePlatformEvent {
	return CodePlatformEvent{EventID: "source-" + value.ID, ReleaseID: value.ID, PullRequestURL: value.PullRequestURL, HeadCommit: value.SourceCommit, MergeCommit: "89abcdef0123456789abcdef0123456789abcdef", ProtectedBranch: true, Approvals: 2, RequiredApprovals: 2, ChecksPassed: true, Merged: true}
}

func successfulEvent(value Request) ProviderEvent {
	checks := make([]CheckEvidence, 0, len(RequiredChecks))
	for _, name := range RequiredChecks {
		checks = append(checks, check(name))
	}
	return ProviderEvent{EventID: "event-success-1", ReleaseID: value.ID, RunID: value.RunID, Phase: ProviderPhaseDeploy, Status: ProviderStatusSucceeded, Artifact: &ArtifactEvidence{URI: "https://artifacts.example.test/order/image", Digest: "sha256:" + hash64("e"), SBOMURI: "https://artifacts.example.test/order/sbom.json", SBOMSHA256: hash64("f"), SignatureVerified: true, ProvenanceVerified: true}, Checks: checks}
}
func check(name string) CheckEvidence {
	return CheckEvidence{Name: name, Status: "passed", EvidenceURI: "https://evidence.example.test/" + name, SHA256: hash64("1")}
}
func hash64(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
