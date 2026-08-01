package agenttask

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestPostgresAgentTaskRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL agent task store: %v", err)
	}
	defer store.Close()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	now := time.Now().UTC()
	task := Task{
		ID: "agent-task-" + suffix, Kind: KindProjectPackage, Step: KindProjectPackage,
		Project: "project-" + suffix, Repository: "repo", Status: StatusPending,
		Input: []byte(`{"session_id":"session"}`), Attempt: 1, TriggeredBy: "approver", CreatedAt: now,
	}
	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("create agent task: %v", err)
	}
	ids, err := store.ListRunnable(ctx, now.Add(time.Second), 10)
	if err != nil || len(ids) == 0 {
		t.Fatalf("list runnable tasks: %#v err=%v", ids, err)
	}
	claimed, ok, err := store.Claim(ctx, task.ID, "worker", now, now.Add(time.Minute))
	if err != nil || !ok || claimed.Status != StatusRunning {
		t.Fatalf("claim agent task: %#v ok=%v err=%v", claimed, ok, err)
	}
	if err := store.RenewLease(ctx, task.ID, "worker", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("renew agent task lease: %v", err)
	}
	claimed.Status = StatusCompleted
	claimed.ResourceID = "package-1"
	claimed.Quality = passedQuality()
	claimed.Usage = Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, CostUSD: 0.01}
	claimed.CompletedAt = now.Add(time.Second)
	if err := store.Complete(ctx, claimed, "worker"); err != nil {
		t.Fatalf("complete agent task: %v", err)
	}
	stored, err := store.Get(ctx, task.ID)
	if err != nil || stored.ResourceID != "package-1" || stored.Usage.TotalTokens != 15 || !stored.Quality.Passed {
		t.Fatalf("get completed agent task: %#v err=%v", stored, err)
	}
	history, err := store.List(ctx, task.Project, KindProjectPackage, StatusCompleted, 10)
	if err != nil || len(history) != 1 || history[0].ID != task.ID {
		t.Fatalf("list agent task history: %#v err=%v", history, err)
	}
	retry := task
	retry.ID = "agent-task-retry-" + suffix
	retry.Status = StatusPending
	retry.RetryOfTaskID = task.ID
	retry.Attempt = 2
	if err := store.Create(ctx, retry); err != nil {
		t.Fatalf("create agent task retry: %v", err)
	}
	retry.ID = "agent-task-duplicate-" + suffix
	if err := store.Create(ctx, retry); !errors.Is(err, ErrTaskAlreadyRetried) {
		t.Fatalf("duplicate retry must fail: %v", err)
	}
}
