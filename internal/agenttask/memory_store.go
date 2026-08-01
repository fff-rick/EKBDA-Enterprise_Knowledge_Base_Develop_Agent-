package agenttask

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]Task
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{tasks: make(map[string]Task)} }

func (s *MemoryStore) Create(_ context.Context, task Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if task.RetryOfTaskID != "" {
		for _, existing := range s.tasks {
			if existing.RetryOfTaskID == task.RetryOfTaskID {
				return ErrTaskAlreadyRetried
			}
		}
	}
	s.tasks[task.ID] = cloneTask(task)
	return nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, found := s.tasks[id]
	if !found {
		return Task{}, ErrTaskNotFound
	}
	return cloneTask(task), nil
}

func (s *MemoryStore) List(_ context.Context, project, kind, status string, limit int) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Task, 0)
	for _, task := range s.tasks {
		if task.Project == project && (kind == "" || task.Kind == kind) && (status == "" || task.Status == status) {
			result = append(result, cloneTask(task))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListRunnable(_ context.Context, now time.Time, limit int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0)
	for _, task := range s.tasks {
		if !task.CancelRequested && (task.Status == StatusPending || (task.Status == StatusRunning && task.LeaseUntil.Before(now))) {
			ids = append(ids, task.ID)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (s *MemoryStore) Claim(_ context.Context, id, workerID string, now, leaseUntil time.Time) (Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, found := s.tasks[id]
	if !found {
		return Task{}, false, ErrTaskNotFound
	}
	claimable := !task.CancelRequested && (task.Status == StatusPending || (task.Status == StatusRunning && task.LeaseUntil.Before(now)))
	if !claimable {
		return cloneTask(task), false, nil
	}
	task.Status = StatusRunning
	task.WorkerID = workerID
	task.LeaseUntil = leaseUntil
	if task.StartedAt.IsZero() {
		task.StartedAt = now
	}
	s.tasks[id] = cloneTask(task)
	return cloneTask(task), true, nil
}

func (s *MemoryStore) RenewLease(_ context.Context, id, workerID string, leaseUntil time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, found := s.tasks[id]
	if !found {
		return ErrTaskNotFound
	}
	if task.Status != StatusRunning || task.WorkerID != workerID {
		return ErrTaskNotClaimed
	}
	task.LeaseUntil = leaseUntil
	s.tasks[id] = cloneTask(task)
	return nil
}

func (s *MemoryStore) RequestCancel(_ context.Context, id string, now time.Time) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, found := s.tasks[id]
	if !found {
		return Task{}, ErrTaskNotFound
	}
	if task.Status == StatusPending {
		task.Status = StatusCanceled
		task.CancelRequested = true
		task.CompletedAt = now
		task.WorkerID = ""
		task.LeaseUntil = time.Time{}
	} else if task.Status == StatusRunning {
		task.CancelRequested = true
	}
	s.tasks[id] = cloneTask(task)
	return cloneTask(task), nil
}

func (s *MemoryStore) Complete(_ context.Context, task Task, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, found := s.tasks[task.ID]
	if !found {
		return ErrTaskNotFound
	}
	if existing.Status != StatusRunning || existing.WorkerID != workerID {
		return ErrTaskNotClaimed
	}
	task.WorkerID = ""
	task.LeaseUntil = time.Time{}
	s.tasks[task.ID] = cloneTask(task)
	return nil
}

func cloneTask(task Task) Task {
	task.Input = append([]byte(nil), task.Input...)
	task.Quality.Checks = append([]QualityCheck(nil), task.Quality.Checks...)
	return task
}
