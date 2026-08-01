package agenttask

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTaskCompletesWithUsageCostAndQuality(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store, Pricing{InputUSDPerMillionTokens: 2, OutputUSDPerMillionTokens: 8}, time.Second)
	defer service.Close()
	if err := service.Register(KindProjectPackage, func(ctx context.Context, task Task) (ExecutionResult, error) {
		RecordUsage(ctx, 1_000_000, 500_000)
		return ExecutionResult{ResourceID: "package-1", Quality: passedQuality()}, nil
	}); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	task, err := service.Create(context.Background(), KindProjectPackage, "order-service", "order-service", map[string]string{"session_id": "session-1"}, "approver-1")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	completed := waitForTask(t, service, task.ID)
	if completed.Status != StatusCompleted || completed.ResourceID != "package-1" || completed.Usage.TotalTokens != 1_500_000 || completed.Usage.CostUSD != 6 {
		t.Fatalf("unexpected completed task: %#v", completed)
	}
}

func TestTaskTimeoutCanBeRetriedOnce(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store, Pricing{}, 20*time.Millisecond)
	defer service.Close()
	if err := service.Register(KindRoleReview, func(ctx context.Context, task Task) (ExecutionResult, error) {
		<-ctx.Done()
		return ExecutionResult{}, ctx.Err()
	}); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	task, err := service.Create(context.Background(), KindRoleReview, "order-service", "order-service", map[string]int{"revision": 2}, "developer-1")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	failed := waitForTask(t, service, task.ID)
	if failed.Status != StatusFailed || failed.ErrorCode != "task_timeout" {
		t.Fatalf("expected timeout: %#v", failed)
	}
	retry, err := service.Retry(context.Background(), failed.ID, "operator-1")
	if err != nil || retry.Attempt != 2 || retry.RetryOfTaskID != failed.ID || retry.TriggeredBy != "developer-1" || retry.RetryRequestedBy != "operator-1" {
		t.Fatalf("create retry: %#v err=%v", retry, err)
	}
	if _, err := service.Retry(context.Background(), failed.ID, "developer-1"); !errors.Is(err, ErrTaskAlreadyRetried) {
		t.Fatalf("duplicate retry must fail: %v", err)
	}
}

func TestTaskCancelAndExpiredLeaseRecovery(t *testing.T) {
	store := NewMemoryStore()
	started := make(chan struct{})
	service := NewService(store, Pricing{}, time.Second)
	defer service.Close()
	if err := service.Register(KindRoleReview, func(ctx context.Context, task Task) (ExecutionResult, error) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done()
		return ExecutionResult{}, ctx.Err()
	}); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	task, err := service.Create(context.Background(), KindRoleReview, "order-service", "order-service", map[string]int{"revision": 2}, "developer-1")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("task did not start")
	}
	if _, err := service.Cancel(context.Background(), task.ID); err != nil {
		t.Fatalf("cancel task: %v", err)
	}
	canceled := waitForTask(t, service, task.ID)
	if canceled.Status != StatusCanceled || !canceled.CancelRequested {
		t.Fatalf("unexpected canceled task: %#v", canceled)
	}

	recoveryStore := NewMemoryStore()
	expired := Task{ID: "expired", Kind: KindProjectPackage, Step: KindProjectPackage, Project: "order-service", Status: StatusRunning, Input: []byte(`{}`), Attempt: 1, WorkerID: "dead", LeaseUntil: time.Now().Add(-time.Minute), TriggeredBy: "approver", CreatedAt: time.Now().Add(-time.Minute)}
	if err := recoveryStore.Create(context.Background(), expired); err != nil {
		t.Fatalf("seed expired task: %v", err)
	}
	recoveryService := NewService(recoveryStore, Pricing{}, time.Second)
	defer recoveryService.Close()
	_ = recoveryService.Register(KindProjectPackage, func(context.Context, Task) (ExecutionResult, error) {
		return ExecutionResult{ResourceID: "package-recovered", Quality: passedQuality()}, nil
	})
	recoveryService.Start()
	recovered := waitForTask(t, recoveryService, expired.ID)
	if recovered.Status != StatusCompleted || recovered.ResourceID != "package-recovered" {
		t.Fatalf("expired lease was not recovered: %#v", recovered)
	}
}

func TestQualityFailureIsNotRetryable(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store, Pricing{}, time.Second)
	defer service.Close()
	_ = service.Register(KindProjectPackage, func(context.Context, Task) (ExecutionResult, error) {
		return ExecutionResult{ResourceID: "package-invalid", Quality: QualityReport{Passed: false, Checks: []QualityCheck{{Name: "traceability", Passed: false}}}}, nil
	})
	task, _ := service.Create(context.Background(), KindProjectPackage, "order-service", "", map[string]string{"session_id": "session"}, "approver")
	failed := waitForTask(t, service, task.ID)
	if failed.ErrorCode != "quality_gate_failed" {
		t.Fatalf("unexpected quality failure: %#v", failed)
	}
	if _, err := service.Retry(context.Background(), failed.ID, "approver"); !errors.Is(err, ErrTaskNotRetryable) {
		t.Fatalf("quality failure must not be retryable: %v", err)
	}
}

func passedQuality() QualityReport {
	return QualityReport{Passed: true, Checks: []QualityCheck{{Name: "schema", Passed: true}}}
}

func waitForTask(t *testing.T, service *Service, id string) Task {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := service.Get(context.Background(), id)
		if err == nil && (task.Status == StatusCompleted || task.Status == StatusFailed || task.Status == StatusCanceled) {
			return task
		}
		time.Sleep(5 * time.Millisecond)
	}
	task, _ := service.Get(context.Background(), id)
	t.Fatalf("task did not finish: %#v", task)
	return Task{}
}
