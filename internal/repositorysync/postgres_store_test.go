package repositorysync

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"ekbda/internal/knowledge"
	"ekbda/internal/workspace"
)

func TestPostgresRepositorySyncReportRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL repository sync store: %v", err)
	}
	defer store.Close()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	report := Report{
		ID: "sync-" + suffix, Status: StatusCompleted, Repository: "repo-" + suffix,
		Project: "project-" + suffix, BusinessDomain: "integration",
		Classification: knowledge.ClassificationInternal,
		HeadCommit:     "0123456789012345678901234567890123456789",
		CommitChanges:  []workspace.CommitChange{{Path: "README.md", Status: "A"}},
		Files:          []FileResult{{Path: "README.md", RedactionCount: 1}}, SyncedBy: "integration",
		StartedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(),
	}
	if err := store.Save(ctx, report); err != nil {
		t.Fatalf("save report: %v", err)
	}
	stored, err := store.Get(ctx, report.ID)
	if err != nil || stored.HeadCommit != report.HeadCommit || len(stored.CommitChanges) != 1 {
		t.Fatalf("get report: %#v err=%v", stored, err)
	}
	latest, err := store.LatestCompleted(ctx, report.Project, report.Repository)
	if err != nil || latest.ID != report.ID {
		t.Fatalf("get latest report: %#v err=%v", latest, err)
	}
}
