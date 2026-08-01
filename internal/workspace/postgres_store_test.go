package workspace

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresWorkspaceSnapshotRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL workspace store: %v", err)
	}
	defer store.Close()
	project := "workspace-" + newID()
	snapshot := Snapshot{
		ID: newID(), Repository: "order-service", Project: project, Technology: "go",
		HeadCommit: "abc123", Branch: "main", Dirty: true, FileCount: 2,
		TrackedCount: 1, UntrackedCount: 1, ChangedCount: 1,
		Changes:   []Change{{Path: "main.go", WorktreeStatus: "M"}},
		InputHash: "hash", StandardsReportID: "report-id", Passed: true,
		ValidatedBy: "integration-test", CreatedAt: time.Now().UTC(),
	}
	if err := store.Save(ctx, snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	stored, err := store.Get(ctx, snapshot.ID)
	if err != nil || stored.HeadCommit != "abc123" || len(stored.Changes) != 1 {
		t.Fatalf("get snapshot: %#v err=%v", stored, err)
	}
	listed, err := store.List(ctx, project, 10)
	if err != nil || len(listed) == 0 || listed[0].ID != snapshot.ID {
		t.Fatalf("list snapshots: %#v err=%v", listed, err)
	}
}
