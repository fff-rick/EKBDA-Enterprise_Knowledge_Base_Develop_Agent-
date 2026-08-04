package release

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreReleaseAndProviderIdempotency(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	request := Request{ID: newID(), Project: "postgres-release-test", Status: StatusAwaitingApproval, Revision: 1, RequiredChecks: append([]string(nil), RequiredChecks...), CreatedAt: now, UpdatedAt: now}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), `DELETE FROM release_requests WHERE id=$1`, request.ID)
	})
	if err := store.Create(ctx, request, Event{ID: newID(), ReleaseID: request.ID, Sequence: 1, Type: "created", ToStatus: request.Status, Actor: "tester", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	request.Status = StatusQueued
	request.Revision = 2
	applied, err := store.ApplyProviderEvent(ctx, request, 1, Event{ID: newID(), ReleaseID: request.ID, Sequence: 2, Type: "provider_deploy_queued", FromStatus: StatusApproved, ToStatus: StatusQueued, Actor: "provider", ProviderEventID: "postgres-event-1", CreatedAt: now}, "postgres-event-1", "payload-hash")
	if err != nil || !applied {
		t.Fatalf("apply provider event: %v applied=%v", err, applied)
	}
	applied, err = store.ApplyProviderEvent(ctx, request, 1, Event{}, "postgres-event-1", "payload-hash")
	if err != nil || applied {
		t.Fatalf("duplicate provider event: %v applied=%v", err, applied)
	}
	loaded, err := store.Get(ctx, request.ID)
	if err != nil || loaded.Status != StatusQueued || loaded.Revision != 2 {
		t.Fatalf("loaded: %#v err=%v", loaded, err)
	}
	events, err := store.ListEvents(ctx, request.ID)
	if err != nil || len(events) != 2 {
		t.Fatalf("events: %#v err=%v", events, err)
	}
}
