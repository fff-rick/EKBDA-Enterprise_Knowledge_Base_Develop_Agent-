package development

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.Close()
	id := newID()
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), `DELETE FROM development_sessions WHERE id=$1`, id)
	})
	now := time.Now().UTC()
	session := Session{ID: id, Project: "postgres-test", Repository: "repo", Status: StatusDraft, Revision: 1, CreatedAt: now, UpdatedAt: now}
	event := Event{ID: newID(), SessionID: id, Sequence: 1, Type: EventCreated, ToStatus: StatusDraft, Actor: "tester", CreatedAt: now}
	if err := store.Create(ctx, session, event); err != nil {
		t.Fatalf("create: %v", err)
	}
	session.Status = StatusExecuting
	session.Revision = 2
	session.Proposal = &Proposal{Patch: validPatch, PatchHash: "hash", SubmittedAt: now}
	if err := store.Update(ctx, session, 1, Event{ID: newID(), SessionID: id, Sequence: 2, Type: EventExecutionStarted, FromStatus: StatusApproved, ToStatus: StatusExecuting, Actor: "tester", CreatedAt: now}); err != nil {
		t.Fatalf("update: %v", err)
	}
	loaded, err := store.Get(ctx, id)
	if err != nil || loaded.Proposal == nil || loaded.Proposal.Patch != validPatch {
		t.Fatalf("get: %#v, %v", loaded, err)
	}
	executing, err := store.ListExecuting(ctx, 10)
	if err != nil || len(executing) == 0 || executing[0].ID != id {
		t.Fatalf("list executing: %#v, %v", executing, err)
	}
	session.Status = StatusDelivering
	session.Revision = 3
	session.Delivery = &Delivery{ID: "delivery-1", Status: DeliveryRunning, Branch: "codex/postgres/test", SecretScan: &SecretScanEvidence{Scanner: "enterprise", Passed: true}, StartedAt: now}
	if err := store.Update(ctx, session, 2, Event{ID: newID(), SessionID: id, Sequence: 3, Type: EventDeliveryStarted, FromStatus: StatusVerified, ToStatus: StatusDelivering, Actor: "tester", CreatedAt: now}); err != nil {
		t.Fatalf("update delivery: %v", err)
	}
	delivering, err := store.ListDelivering(ctx, 10)
	if err != nil || len(delivering) == 0 || delivering[0].ID != id || delivering[0].Delivery == nil || delivering[0].Delivery.SecretScan == nil {
		t.Fatalf("list delivering: %#v, %v", delivering, err)
	}
	events, err := store.ListEvents(ctx, id)
	if err != nil || len(events) != 3 {
		t.Fatalf("events: %#v, %v", events, err)
	}
}
