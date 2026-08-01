package ingestion

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresJobStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := NewPostgresJobStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL job store: %v", err)
	}
	defer store.Close()

	report := Report{ID: newID(), Status: "running", Root: "test", Project: "test", StartedAt: time.Now().UTC()}
	if err := store.Create(ctx, report); err != nil {
		t.Fatalf("create job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), "DELETE FROM ingestion_jobs WHERE id = $1", report.ID)
	})
	file := FileResult{Path: "README.md", Action: "created", DocumentID: "document", Version: 1}
	report.Status = "completed"
	report.Created = 1
	report.CompletedAt = time.Now().UTC()
	if err := store.Update(ctx, report, &file); err != nil {
		t.Fatalf("update job: %v", err)
	}
	stored, err := store.Get(ctx, report.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if stored.Status != "completed" || stored.Created != 1 || len(stored.Files) != 1 {
		t.Fatalf("unexpected stored job: %#v", stored)
	}
}
