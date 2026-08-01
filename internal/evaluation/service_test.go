package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	"ekbda/internal/answer"
	"ekbda/internal/embedding"
	"ekbda/internal/knowledge"
)

func TestCreateSuiteAllocatesImmutableVersions(t *testing.T) {
	service := newEvaluationService(t)
	threshold := 0.8
	input := CreateSuiteInput{
		Name:            "release-gate",
		MinimumPassRate: &threshold,
		Cases: []Case{{
			Name:    "startup",
			Query:   "go run ./cmd/server",
			Project: "app",
		}},
	}
	first, err := service.CreateSuite(context.Background(), input, "admin-1")
	if err != nil {
		t.Fatalf("create first suite: %v", err)
	}
	input.Cases[0].Query = "mutated after creation"
	second, err := service.CreateSuite(context.Background(), input, "admin-1")
	if err != nil {
		t.Fatalf("create second suite: %v", err)
	}
	stored, err := service.GetSuite(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("get first suite: %v", err)
	}
	if first.Version != 1 || second.Version != 2 || stored.Cases[0].Query != "go run ./cmd/server" {
		t.Fatalf("unexpected suite versions or mutation: first=%#v second=%#v stored=%#v", first, second, stored)
	}
	if first.DefinitionHash == second.DefinitionHash {
		t.Fatal("different suite definitions must have different hashes")
	}
}

func TestAsyncRunPersistsPassedAndFailedGates(t *testing.T) {
	service := newEvaluationService(t)
	passThreshold := 1.0
	passedSuite, err := service.CreateSuite(context.Background(), CreateSuiteInput{
		Name:            "passing-gate",
		MinimumPassRate: &passThreshold,
		Cases: []Case{{
			Name:            "startup",
			Query:           "go run ./cmd/server",
			Project:         "app",
			RequiredSources: []string{"git://app/README.md"},
		}},
	}, "admin-1")
	if err != nil {
		t.Fatalf("create passing suite: %v", err)
	}
	passed := startAndWait(t, service, passedSuite.ID)
	if passed.Status != RunCompleted || passed.GateStatus != GatePassed || passed.Report.PassRate != 1 {
		t.Fatalf("unexpected passing run: %#v", passed)
	}

	failedSuite, err := service.CreateSuite(context.Background(), CreateSuiteInput{
		Name:            "failing-gate",
		MinimumPassRate: &passThreshold,
		Cases: []Case{{
			Name:    "missing project should answer",
			Query:   "answer this",
			Project: "missing-project",
		}},
	}, "admin-1")
	if err != nil {
		t.Fatalf("create failing suite: %v", err)
	}
	failed := startAndWait(t, service, failedSuite.ID)
	if failed.Status != RunCompleted || failed.GateStatus != GateFailed || failed.Report.Failed != 1 {
		t.Fatalf("unexpected failed gate: %#v", failed)
	}
	history, err := service.ListRuns(context.Background(), failedSuite.ID, 10)
	if err != nil || len(history) != 1 || history[0].ID != failed.ID {
		t.Fatalf("unexpected run history: %#v, %v", history, err)
	}
	if _, err := service.RetryRun(context.Background(), failed.ID, "ci"); !errors.Is(err, ErrRunNotRetryable) {
		t.Fatalf("quality failures must not be retryable, got %v", err)
	}
}

func TestCancelRunningEvaluationAndCreateRetry(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title: "Startup", Content: "Run the service.", SourceURI: "git://app/README.md", Project: "app",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	answerService := answer.NewService(knowledgeService, blockingProvider{}, answer.NewMemoryTraceStore())
	service := NewService(NewRunner(answerService), NewMemoryStore())
	t.Cleanup(service.Close)
	suite, err := service.CreateSuite(context.Background(), CreateSuiteInput{
		Name: "cancel", Cases: []Case{{Name: "case", Query: "Run the service", Project: "app"}},
	}, "admin")
	if err != nil {
		t.Fatalf("create suite: %v", err)
	}
	started, err := service.StartRun(context.Background(), StartRunInput{SuiteID: suite.ID}, "ci")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	waitForStatus(t, service, started.ID, RunRunning)
	if _, err := service.CancelRun(context.Background(), started.ID); err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	canceled := waitForStatus(t, service, started.ID, RunCanceled)
	if canceled.GateStatus != GateCanceled || !canceled.CancelRequested {
		t.Fatalf("unexpected canceled run: %#v", canceled)
	}
	retry, err := service.RetryRun(context.Background(), canceled.ID, "ci-retry")
	if err != nil {
		t.Fatalf("retry canceled run: %v", err)
	}
	if retry.Attempt != 2 || retry.RetryOfRunID != canceled.ID {
		t.Fatalf("unexpected retry lineage: %#v", retry)
	}
	if _, err := service.RetryRun(context.Background(), canceled.ID, "duplicate-retry"); !errors.Is(err, ErrRunAlreadyRetried) {
		t.Fatalf("expected duplicate retry rejection, got %v", err)
	}
	_, _ = service.CancelRun(context.Background(), retry.ID)
}

func TestWorkerRecoversExpiredLease(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title: "Startup", Content: "Run go run ./cmd/server.", SourceURI: "git://app/README.md", Project: "app",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	store := NewMemoryStore()
	answerService := answer.NewService(knowledgeService, answer.NewLocalExtractive(), answer.NewMemoryTraceStore())
	service := NewService(NewRunner(answerService), store)
	t.Cleanup(service.Close)
	suite, err := service.CreateSuite(context.Background(), CreateSuiteInput{
		Name: "recover", Cases: []Case{{Name: "case", Query: "go run ./cmd/server", Project: "app"}},
	}, "admin")
	if err != nil {
		t.Fatalf("create suite: %v", err)
	}
	run := Run{
		ID: newID(), SuiteID: suite.ID, SuiteName: suite.Name, SuiteVersion: suite.Version,
		DefinitionHash: suite.DefinitionHash, MinimumPassRate: suite.MinimumPassRate,
		Status: RunRunning, GateStatus: GatePending, Attempt: 1, WorkerID: "dead-worker",
		LeaseUntil: time.Now().UTC().Add(-time.Minute), CreatedAt: time.Now().UTC().Add(-time.Minute),
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create expired run: %v", err)
	}
	service.Start()
	recovered := waitForStatus(t, service, run.ID, RunCompleted)
	if recovered.GateStatus != GatePassed || recovered.WorkerID != "" {
		t.Fatalf("unexpected recovered run: %#v", recovered)
	}
}

func TestRetryStopsAtThreeAttempts(t *testing.T) {
	service := newEvaluationService(t)
	store := service.store.(*MemoryStore)
	run := Run{ID: newID(), Status: RunFailed, GateStatus: GateError, Attempt: 3, CreatedAt: time.Now().UTC()}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create failed run: %v", err)
	}
	if _, err := service.RetryRun(context.Background(), run.ID, "ci"); !errors.Is(err, ErrRunNotRetryable) {
		t.Fatalf("expected retry limit, got %v", err)
	}
}

func TestCreateSuiteRejectsInvalidThreshold(t *testing.T) {
	service := newEvaluationService(t)
	threshold := 1.1
	_, err := service.CreateSuite(context.Background(), CreateSuiteInput{
		Name:            "invalid",
		MinimumPassRate: &threshold,
		Cases:           []Case{{Name: "case", Query: "query", Project: "project"}},
	}, "admin-1")
	if !errors.Is(err, ErrInvalidSuiteInput) {
		t.Fatalf("expected invalid suite input, got %v", err)
	}
}

func newEvaluationService(t *testing.T) *Service {
	t.Helper()
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title:     "Startup",
		Content:   "Run go run ./cmd/server to start the app.",
		SourceURI: "git://app/README.md",
		Project:   "app",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	answerService := answer.NewService(knowledgeService, answer.NewLocalExtractive(), answer.NewMemoryTraceStore())
	service := NewService(NewRunner(answerService), NewMemoryStore())
	t.Cleanup(service.Close)
	return service
}

func startAndWait(t *testing.T, service *Service, suiteID string) Run {
	t.Helper()
	started, err := service.StartRun(context.Background(), StartRunInput{SuiteID: suiteID}, "ci")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(context.Background(), started.ID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if run.Status == RunCompleted || run.Status == RunFailed || run.Status == RunCanceled {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("evaluation run did not complete")
	return Run{}
}

func waitForStatus(t *testing.T, service *Service, runID, status string) Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if run.Status == status {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not reach status %s", status)
	return Run{}
}

type blockingProvider struct{}

func (blockingProvider) Name() string { return "blocking" }

func (blockingProvider) Generate(ctx context.Context, _ string, _ []answer.Evidence) (answer.Draft, error) {
	<-ctx.Done()
	return answer.Draft{}, ctx.Err()
}
