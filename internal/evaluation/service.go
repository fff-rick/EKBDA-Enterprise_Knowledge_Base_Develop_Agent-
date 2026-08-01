package evaluation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidSuiteInput = errors.New("name, 1 to 100 valid cases and minimum_pass_rate between 0 and 1 are required")
	ErrRunNotRetryable   = errors.New("only failed or canceled runs may be retried, up to three attempts")
)

const (
	defaultMinimumPassRate = 0.9
	maxRunAttempts         = 3
	runLeaseDuration       = 30 * time.Second
	runHeartbeatInterval   = 10 * time.Second
	runRecoveryInterval    = 5 * time.Second
)

type Service struct {
	runner    *Runner
	store     Store
	workerID  string
	queue     chan string
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	wg        sync.WaitGroup
	activeMu  sync.Mutex
	active    map[string]context.CancelFunc
}

func NewService(runner *Runner, store Store) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		runner: runner, store: store, workerID: newID(), queue: make(chan string, 100),
		ctx: ctx, cancel: cancel, active: make(map[string]context.CancelFunc),
	}
}

func (s *Service) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.worker()
	})
}

func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

func (s *Service) RunAdHoc(ctx context.Context, request Request) (Report, error) {
	return s.runner.Run(ctx, request)
}

func (s *Service) CreateSuite(ctx context.Context, input CreateSuiteInput, createdBy string) (Suite, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	minimumPassRate := defaultMinimumPassRate
	if input.MinimumPassRate != nil {
		minimumPassRate = *input.MinimumPassRate
	}
	if input.Name == "" || minimumPassRate < 0 || minimumPassRate > 1 || validate(Request{Cases: input.Cases}) != nil {
		return Suite{}, ErrInvalidSuiteInput
	}
	definition, err := json.Marshal(struct {
		MinimumPassRate float64 `json:"minimum_pass_rate"`
		Cases           []Case  `json:"cases"`
	}{MinimumPassRate: minimumPassRate, Cases: input.Cases})
	if err != nil {
		return Suite{}, fmt.Errorf("encode evaluation suite definition: %w", err)
	}
	hash := sha256.Sum256(definition)
	return s.store.CreateSuite(ctx, Suite{
		ID:              newID(),
		Name:            input.Name,
		Description:     input.Description,
		DefinitionHash:  hex.EncodeToString(hash[:]),
		MinimumPassRate: minimumPassRate,
		Cases:           cloneCases(input.Cases),
		CreatedBy:       strings.TrimSpace(createdBy),
		CreatedAt:       time.Now().UTC(),
	})
}

func (s *Service) GetSuite(ctx context.Context, id string) (Suite, error) {
	return s.store.GetSuite(ctx, strings.TrimSpace(id))
}

func (s *Service) ListSuites(ctx context.Context, name string, limit int) ([]Suite, error) {
	return s.store.ListSuites(ctx, name, normalizeLimit(limit))
}

func (s *Service) StartRun(ctx context.Context, input StartRunInput, triggeredBy string) (Run, error) {
	suite, err := s.store.GetSuite(ctx, strings.TrimSpace(input.SuiteID))
	if err != nil {
		return Run{}, err
	}
	run := Run{
		ID:              newID(),
		SuiteID:         suite.ID,
		SuiteName:       suite.Name,
		SuiteVersion:    suite.Version,
		DefinitionHash:  suite.DefinitionHash,
		MinimumPassRate: suite.MinimumPassRate,
		Status:          RunPending,
		GateStatus:      GatePending,
		Attempt:         1,
		TriggeredBy:     strings.TrimSpace(triggeredBy),
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return Run{}, fmt.Errorf("create evaluation run: %w", err)
	}
	s.Start()
	s.enqueue(run.ID)
	return run, nil
}

func (s *Service) GetRun(ctx context.Context, id string) (Run, error) {
	return s.store.GetRun(ctx, strings.TrimSpace(id))
}

func (s *Service) ListRuns(ctx context.Context, suiteID string, limit int) ([]Run, error) {
	return s.store.ListRuns(ctx, suiteID, normalizeLimit(limit))
}

func (s *Service) CancelRun(ctx context.Context, id string) (Run, error) {
	run, err := s.store.RequestCancel(ctx, strings.TrimSpace(id), time.Now().UTC())
	if err != nil {
		return Run{}, err
	}
	s.activeMu.Lock()
	cancel := s.active[run.ID]
	s.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return run, nil
}

func (s *Service) RetryRun(ctx context.Context, id, triggeredBy string) (Run, error) {
	original, err := s.store.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return Run{}, err
	}
	attempt := original.Attempt
	if attempt < 1 {
		attempt = 1
	}
	if (original.Status != RunFailed && original.Status != RunCanceled) || attempt >= maxRunAttempts {
		return Run{}, ErrRunNotRetryable
	}
	run := Run{
		ID: newID(), SuiteID: original.SuiteID, SuiteName: original.SuiteName,
		SuiteVersion: original.SuiteVersion, DefinitionHash: original.DefinitionHash,
		MinimumPassRate: original.MinimumPassRate, Status: RunPending,
		GateStatus: GatePending, RetryOfRunID: original.ID, Attempt: attempt + 1,
		TriggeredBy: strings.TrimSpace(triggeredBy), CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return Run{}, fmt.Errorf("create evaluation retry: %w", err)
	}
	s.Start()
	s.enqueue(run.ID)
	return run, nil
}

func (s *Service) worker() {
	defer s.wg.Done()
	s.recoverRunnable()
	ticker := time.NewTicker(runRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case id := <-s.queue:
			if s.ctx.Err() != nil {
				return
			}
			s.process(id)
		case <-ticker.C:
			s.recoverRunnable()
		}
	}
}

func (s *Service) enqueue(id string) {
	select {
	case s.queue <- id:
	default:
	}
}

func (s *Service) recoverRunnable() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	ids, err := s.store.ListRunnable(ctx, time.Now().UTC(), 100)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.enqueue(id)
	}
}

func (s *Service) process(id string) {
	now := time.Now().UTC()
	claimContext, cancelClaim := context.WithTimeout(s.ctx, 5*time.Second)
	run, claimed, err := s.store.ClaimRun(claimContext, id, s.workerID, now, now.Add(runLeaseDuration))
	cancelClaim()
	if err != nil || !claimed {
		return
	}
	suite, err := s.store.GetSuite(s.ctx, run.SuiteID)
	if err != nil {
		s.completeFailed(run, "suite_unavailable")
		return
	}
	workContext, cancelWork := context.WithTimeout(s.ctx, 30*time.Minute)
	s.activeMu.Lock()
	s.active[run.ID] = cancelWork
	s.activeMu.Unlock()
	cancelCheckContext, cancelCheck := context.WithTimeout(context.Background(), 5*time.Second)
	current, currentErr := s.store.GetRun(cancelCheckContext, run.ID)
	cancelCheck()
	if currentErr == nil && current.CancelRequested {
		cancelWork()
	}
	heartbeatDone := make(chan struct{})
	go s.heartbeat(run.ID, cancelWork, heartbeatDone)
	report, runErr := s.runner.Run(workContext, Request{Cases: cloneCases(suite.Cases), UserID: run.TriggeredBy})
	close(heartbeatDone)
	s.activeMu.Lock()
	delete(s.active, run.ID)
	s.activeMu.Unlock()
	if s.ctx.Err() != nil {
		cancelWork()
		return
	}
	latestContext, cancelLatest := context.WithTimeout(context.Background(), 5*time.Second)
	latest, getErr := s.store.GetRun(latestContext, run.ID)
	cancelLatest()
	run.CompletedAt = time.Now().UTC()
	switch {
	case getErr == nil && latest.CancelRequested:
		run.Status = RunCanceled
		run.GateStatus = GateCanceled
		run.CancelRequested = true
	case errors.Is(workContext.Err(), context.DeadlineExceeded):
		run.Status = RunFailed
		run.GateStatus = GateError
		run.ErrorCode = "evaluation_timeout"
	case errors.Is(workContext.Err(), context.Canceled):
		run.Status = RunFailed
		run.GateStatus = GateError
		run.ErrorCode = "lease_lost"
	case runErr != nil:
		run.Status = RunFailed
		run.GateStatus = GateError
		run.ErrorCode = "evaluation_failed"
	default:
		run.Report = report
		run.Status = RunCompleted
		if report.PassRate >= run.MinimumPassRate {
			run.GateStatus = GatePassed
		} else {
			run.GateStatus = GateFailed
		}
	}
	cancelWork()
	completeContext, cancelComplete := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelComplete()
	_ = s.store.CompleteRun(completeContext, run, s.workerID)
}

func (s *Service) heartbeat(id string, cancelWork context.CancelFunc, done <-chan struct{}) {
	ticker := time.NewTicker(runHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			err := s.store.RenewLease(ctx, id, s.workerID, time.Now().UTC().Add(runLeaseDuration))
			cancel()
			if err != nil {
				cancelWork()
				return
			}
		}
	}
}

func (s *Service) completeFailed(run Run, errorCode string) {
	run.Status = RunFailed
	run.GateStatus = GateError
	run.ErrorCode = errorCode
	run.CompletedAt = time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.store.CompleteRun(ctx, run, s.workerID)
}

func normalizeLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 50
	}
	return limit
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("evaluation-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
