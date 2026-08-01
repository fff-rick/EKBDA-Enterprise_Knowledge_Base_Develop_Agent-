package evaluation

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryStore struct {
	mu     sync.RWMutex
	suites map[string]Suite
	runs   map[string]Run
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		suites: make(map[string]Suite),
		runs:   make(map[string]Run),
	}
}

func (s *MemoryStore) CreateSuite(_ context.Context, suite Suite) (Suite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version := 1
	for _, existing := range s.suites {
		if existing.Name == suite.Name && existing.Version >= version {
			version = existing.Version + 1
		}
	}
	suite.Version = version
	s.suites[suite.ID] = cloneSuite(suite)
	return cloneSuite(suite), nil
}

func (s *MemoryStore) GetSuite(_ context.Context, id string) (Suite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	suite, exists := s.suites[id]
	if !exists {
		return Suite{}, ErrSuiteNotFound
	}
	return cloneSuite(suite), nil
}

func (s *MemoryStore) ListSuites(_ context.Context, name string, limit int) ([]Suite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name = strings.TrimSpace(name)
	result := make([]Suite, 0)
	for _, suite := range s.suites {
		if name == "" || suite.Name == name {
			result = append(result, cloneSuite(suite))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].Version > result[j].Version
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) CreateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.RetryOfRunID != "" {
		for _, existing := range s.runs {
			if existing.RetryOfRunID == run.RetryOfRunID {
				return ErrRunAlreadyRetried
			}
		}
	}
	if run.Attempt < 1 {
		run.Attempt = 1
	}
	s.runs[run.ID] = cloneRun(run)
	return nil
}

func (s *MemoryStore) UpdateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; !exists {
		return ErrRunNotFound
	}
	s.runs[run.ID] = cloneRun(run)
	return nil
}

func (s *MemoryStore) GetRun(_ context.Context, id string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, exists := s.runs[id]
	if !exists {
		return Run{}, ErrRunNotFound
	}
	return cloneRun(run), nil
}

func (s *MemoryStore) ListRuns(_ context.Context, suiteID string, limit int) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	suiteID = strings.TrimSpace(suiteID)
	result := make([]Run, 0)
	for _, run := range s.runs {
		if suiteID == "" || run.SuiteID == suiteID {
			result = append(result, cloneRun(run))
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
	result := make([]string, 0)
	for _, run := range s.runs {
		if run.CancelRequested {
			continue
		}
		if run.Status == RunPending || (run.Status == RunRunning && run.LeaseUntil.Before(now)) {
			result = append(result, run.ID)
		}
	}
	sort.Strings(result)
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ClaimRun(_ context.Context, id, workerID string, now, leaseUntil time.Time) (Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.runs[id]
	if !exists {
		return Run{}, false, ErrRunNotFound
	}
	claimable := !run.CancelRequested && (run.Status == RunPending || (run.Status == RunRunning && run.LeaseUntil.Before(now)))
	if !claimable {
		return cloneRun(run), false, nil
	}
	run.Status = RunRunning
	run.WorkerID = workerID
	run.LeaseUntil = leaseUntil
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	s.runs[id] = cloneRun(run)
	return cloneRun(run), true, nil
}

func (s *MemoryStore) RenewLease(_ context.Context, id, workerID string, leaseUntil time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.runs[id]
	if !exists {
		return ErrRunNotFound
	}
	if run.Status != RunRunning || run.WorkerID != workerID {
		return ErrRunNotClaimed
	}
	run.LeaseUntil = leaseUntil
	s.runs[id] = cloneRun(run)
	return nil
}

func (s *MemoryStore) RequestCancel(_ context.Context, id string, now time.Time) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.runs[id]
	if !exists {
		return Run{}, ErrRunNotFound
	}
	if run.Status == RunPending || run.Status == RunRunning {
		run.Status = RunCanceled
		run.GateStatus = GateCanceled
		run.CompletedAt = now
		run.CancelRequested = true
		run.WorkerID = ""
		run.LeaseUntil = time.Time{}
	}
	s.runs[id] = cloneRun(run)
	return cloneRun(run), nil
}

func (s *MemoryStore) CompleteRun(_ context.Context, run Run, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, exists := s.runs[run.ID]
	if !exists {
		return ErrRunNotFound
	}
	if existing.Status != RunRunning || existing.WorkerID != workerID {
		return ErrRunNotClaimed
	}
	run.WorkerID = ""
	run.LeaseUntil = time.Time{}
	s.runs[run.ID] = cloneRun(run)
	return nil
}

func cloneSuite(suite Suite) Suite {
	suite.Cases = cloneCases(suite.Cases)
	return suite
}

func cloneRun(run Run) Run {
	run.Report.Results = append([]CaseResult(nil), run.Report.Results...)
	for index := range run.Report.Results {
		run.Report.Results[index].Failures = append([]string(nil), run.Report.Results[index].Failures...)
		run.Report.Results[index].CitationSources = append([]string(nil), run.Report.Results[index].CitationSources...)
	}
	return run
}

func cloneCases(cases []Case) []Case {
	result := append([]Case(nil), cases...)
	for index := range result {
		result[index].Roles = append([]string(nil), result[index].Roles...)
		result[index].RequiredSources = append([]string(nil), result[index].RequiredSources...)
		result[index].AnswerContains = append([]string(nil), result[index].AnswerContains...)
	}
	return result
}
