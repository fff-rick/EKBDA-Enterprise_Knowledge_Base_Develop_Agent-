package initiative

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"ekbda/internal/planning"
)

func TestPostgresProjectPackageRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	planningStore, err := planning.NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL planning store: %v", err)
	}
	defer planningStore.Close()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL project package store: %v", err)
	}
	defer store.Close()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	now := time.Now().UTC()
	session := planning.Session{ID: "package-session-" + suffix, Project: "project-" + suffix, Repository: "repo", Technology: "go", Title: "test", Requirement: "approved test requirement", Status: planning.StatusApproved, Revision: 1, Context: planning.ContextSnapshot{Hash: "hash"}, Provider: "test", CreatedBy: "creator", CreatedAt: now, UpdatedAt: now}
	if err := planningStore.Create(ctx, session, planning.Event{ID: "package-event-" + suffix, SessionID: session.ID, Sequence: 1, Type: "created", ToStatus: session.Status, Actor: "creator", CreatedAt: now}); err != nil {
		t.Fatalf("seed planning session: %v", err)
	}
	projectPackage := Package{
		ID: "package-" + suffix, Project: session.Project, Repository: "repo", Name: "initiative", DefinitionHash: "hash", Provider: "test",
		Source: SourceSnapshot{PlanningSessionID: session.ID, PlanningRevision: 1}, Artifacts: []Artifact{}, CreatedBy: "approver", CreatedAt: now,
	}
	first, err := store.Create(ctx, projectPackage)
	if err != nil || first.Version != 1 {
		t.Fatalf("create project package: %#v err=%v", first, err)
	}
	projectPackage.ID = "package-second-" + suffix
	second, err := store.Create(ctx, projectPackage)
	if err != nil || second.Version != 2 {
		t.Fatalf("create second project package: %#v err=%v", second, err)
	}
	stored, err := store.Get(ctx, first.ID)
	if err != nil || stored.Version != 1 || stored.Source.PlanningSessionID != session.ID {
		t.Fatalf("get project package: %#v err=%v", stored, err)
	}
	history, err := store.List(ctx, session.Project, "initiative", 10)
	if err != nil || len(history) != 2 || history[0].Version != 2 {
		t.Fatalf("list project package history: %#v err=%v", history, err)
	}
	review, err := store.CreateReview(ctx, ArtifactReview{ID: "review-" + suffix, PackageID: first.ID, ArtifactType: ArtifactPRD, PackageHash: first.DefinitionHash, Decision: "approve", Comment: "approved", ReviewedBy: "approver", ReviewedAt: now})
	if err != nil || review.Sequence != 1 {
		t.Fatalf("create project package review: %#v err=%v", review, err)
	}
	reviews, err := store.ListReviews(ctx, first.ID, ArtifactPRD, 10)
	if err != nil || len(reviews) != 1 || reviews[0].ID != review.ID {
		t.Fatalf("list project package reviews: %#v err=%v", reviews, err)
	}
}
