package agenttask

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidInput        = errors.New("invalid agent task input")
	ErrExecutorUnavailable = errors.New("agent task executor is unavailable")
	ErrTaskNotRetryable    = errors.New("only failed or canceled agent tasks may be retried, up to three attempts")
)

const (
	maxTaskAttempts       = 3
	taskLeaseDuration     = 30 * time.Second
	taskHeartbeatInterval = 10 * time.Second
	taskRecoveryInterval  = 5 * time.Second
)

var taskKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type Executor func(context.Context, Task) (ExecutionResult, error)

type Service struct {
	store     Store
	pricing   Pricing
	timeout   time.Duration
	executors map[string]Executor
	workerID  string
	queue     chan string
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	wg        sync.WaitGroup
	activeMu  sync.Mutex
	active    map[string]context.CancelFunc
}

func NewService(store Store, pricing Pricing, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		store: store, pricing: pricing, timeout: timeout, executors: make(map[string]Executor),
		workerID: newID(), queue: make(chan string, 100), ctx: ctx, cancel: cancel,
		active: make(map[string]context.CancelFunc),
	}
}

func (s *Service) Register(kind string, executor Executor) error {
	kind = strings.TrimSpace(kind)
	if !validKind(kind) || executor == nil {
		return ErrInvalidInput
	}
	s.executors[kind] = executor
	return nil
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

func (s *Service) Create(ctx context.Context, kind, project, repository string, input any, actor string) (Task, error) {
	kind = strings.TrimSpace(kind)
	project = strings.ToLower(strings.TrimSpace(project))
	repository = strings.ToLower(strings.TrimSpace(repository))
	actor = strings.TrimSpace(actor)
	if !validKind(kind) || !taskKeyPattern.MatchString(project) || (repository != "" && !taskKeyPattern.MatchString(repository)) || actor == "" || len(actor) > 256 {
		return Task{}, ErrInvalidInput
	}
	if s.executors[kind] == nil {
		return Task{}, ErrExecutorUnavailable
	}
	payload, err := json.Marshal(input)
	if err != nil || len(payload) == 0 || len(payload) > 1<<20 {
		return Task{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	task := Task{
		ID: newID(), Kind: kind, Step: kind, Project: project, Repository: repository,
		Status: StatusPending, Input: payload, Attempt: 1, TriggeredBy: actor, CreatedAt: now,
	}
	if err := s.store.Create(ctx, task); err != nil {
		return Task{}, fmt.Errorf("create agent task: %w", err)
	}
	s.Start()
	s.enqueue(task.ID)
	return task, nil
}

func (s *Service) Get(ctx context.Context, id string) (Task, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, project, kind, status string, limit int) ([]Task, error) {
	project = strings.ToLower(strings.TrimSpace(project))
	kind = strings.TrimSpace(kind)
	status = strings.TrimSpace(status)
	if !taskKeyPattern.MatchString(project) || (kind != "" && !validKind(kind)) || (status != "" && !validStatus(status)) {
		return nil, ErrInvalidInput
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.List(ctx, project, kind, status, limit)
}

func (s *Service) Cancel(ctx context.Context, id string) (Task, error) {
	task, err := s.store.RequestCancel(ctx, strings.TrimSpace(id), time.Now().UTC())
	if err != nil {
		return Task{}, err
	}
	s.activeMu.Lock()
	cancel := s.active[task.ID]
	s.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return task, nil
}

func (s *Service) Retry(ctx context.Context, id, actor string) (Task, error) {
	original, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Task{}, err
	}
	actor = strings.TrimSpace(actor)
	if actor == "" || (original.Status != StatusFailed && original.Status != StatusCanceled) || original.Attempt >= maxTaskAttempts || original.ErrorCode == "quality_gate_failed" || s.executors[original.Kind] == nil {
		return Task{}, ErrTaskNotRetryable
	}
	task := Task{
		ID: newID(), Kind: original.Kind, Step: original.Step, Project: original.Project,
		Repository: original.Repository, Status: StatusPending, Input: append([]byte(nil), original.Input...),
		RetryOfTaskID: original.ID, Attempt: original.Attempt + 1, TriggeredBy: original.TriggeredBy,
		RetryRequestedBy: actor, CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Create(ctx, task); err != nil {
		return Task{}, fmt.Errorf("create agent task retry: %w", err)
	}
	s.Start()
	s.enqueue(task.ID)
	return task, nil
}

func (s *Service) worker() {
	defer s.wg.Done()
	s.recoverRunnable()
	ticker := time.NewTicker(taskRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case id := <-s.queue:
			if s.ctx.Err() == nil {
				s.process(id)
			}
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
	task, claimed, err := s.store.Claim(claimContext, id, s.workerID, now, now.Add(taskLeaseDuration))
	cancelClaim()
	if err != nil || !claimed {
		return
	}
	executor := s.executors[task.Kind]
	if executor == nil {
		s.completeFailed(task, "executor_unavailable")
		return
	}
	workContext, cancelWork := context.WithTimeout(s.ctx, s.timeout)
	collector := &UsageCollector{}
	workContext = WithUsageCollector(workContext, collector)
	s.activeMu.Lock()
	s.active[task.ID] = cancelWork
	s.activeMu.Unlock()
	heartbeatDone := make(chan struct{})
	go s.heartbeat(task.ID, cancelWork, heartbeatDone)
	result, runErr := executor(workContext, task)
	close(heartbeatDone)
	s.activeMu.Lock()
	delete(s.active, task.ID)
	s.activeMu.Unlock()
	if s.ctx.Err() != nil {
		cancelWork()
		return
	}
	latestContext, cancelLatest := context.WithTimeout(context.Background(), 5*time.Second)
	latest, getErr := s.store.Get(latestContext, task.ID)
	cancelLatest()
	task.CompletedAt = time.Now().UTC()
	task.Usage = collector.Snapshot(s.pricing)
	switch {
	case getErr == nil && latest.CancelRequested:
		task.Status = StatusCanceled
		task.CancelRequested = true
	case errors.Is(workContext.Err(), context.DeadlineExceeded):
		task.Status = StatusFailed
		task.ErrorCode = "task_timeout"
	case errors.Is(workContext.Err(), context.Canceled):
		task.Status = StatusFailed
		task.ErrorCode = "lease_lost"
	case runErr != nil:
		task.Status = StatusFailed
		task.ErrorCode = "execution_failed"
	case !result.Quality.Passed:
		task.Status = StatusFailed
		task.ErrorCode = "quality_gate_failed"
		task.ResourceID = result.ResourceID
		task.Quality = result.Quality
	default:
		task.Status = StatusCompleted
		task.ResourceID = result.ResourceID
		task.Quality = result.Quality
	}
	cancelWork()
	completeContext, cancelComplete := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelComplete()
	_ = s.store.Complete(completeContext, task, s.workerID)
}

func (s *Service) heartbeat(id string, cancelWork context.CancelFunc, done <-chan struct{}) {
	ticker := time.NewTicker(taskHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			err := s.store.RenewLease(ctx, id, s.workerID, time.Now().UTC().Add(taskLeaseDuration))
			cancel()
			if err != nil {
				cancelWork()
				return
			}
		}
	}
}

func (s *Service) completeFailed(task Task, errorCode string) {
	task.Status = StatusFailed
	task.ErrorCode = errorCode
	task.CompletedAt = time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.store.Complete(ctx, task, s.workerID)
}

func validKind(kind string) bool {
	return kind == KindRoleReview || kind == KindProjectPackage
}

func validStatus(status string) bool {
	return status == StatusPending || status == StatusRunning || status == StatusCompleted || status == StatusFailed || status == StatusCanceled
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("agent-task-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
