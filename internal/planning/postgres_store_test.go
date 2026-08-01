package planning

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestPostgresPlanningSessionRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL planning store: %v", err)
	}
	defer store.Close()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	now := time.Now().UTC()
	session := Session{ID: "plan-" + suffix, Project: "project-" + suffix, Repository: "repo", Technology: "go", Title: "test", Requirement: "test requirement", Status: StatusAwaitingClarification, Revision: 1, Questions: []Question{{ID: "scope", Question: "scope?"}}, Context: ContextSnapshot{Hash: "hash"}, Provider: "test", CreatedBy: "creator", CreatedAt: now, UpdatedAt: now}
	event := Event{ID: "event-" + suffix, SessionID: session.ID, Sequence: 1, Type: "created", ToStatus: session.Status, Actor: "creator", CreatedAt: now}
	if err := store.Create(ctx, session, event); err != nil {
		t.Fatalf("create planning session: %v", err)
	}
	session.Status = StatusAwaitingApproval
	session.Revision = 2
	session.UpdatedAt = time.Now().UTC()
	updateEvent := Event{ID: "event-update-" + suffix, SessionID: session.ID, Sequence: 2, Type: "clarifications_submitted", FromStatus: StatusAwaitingClarification, ToStatus: session.Status, Actor: "creator", CreatedAt: session.UpdatedAt}
	if err := store.Update(ctx, session, 1, updateEvent); err != nil {
		t.Fatalf("update planning session: %v", err)
	}
	stored, err := store.Get(ctx, session.ID)
	if err != nil || stored.Revision != 2 || stored.Status != StatusAwaitingApproval {
		t.Fatalf("get planning session: %#v err=%v", stored, err)
	}
	events, err := store.ListEvents(ctx, session.ID)
	if err != nil || len(events) != 2 || events[1].Sequence != 2 {
		t.Fatalf("list planning events: %#v err=%v", events, err)
	}
}
