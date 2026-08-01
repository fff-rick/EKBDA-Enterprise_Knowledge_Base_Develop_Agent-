package ingestion

import (
	"context"
	"sync"
)

type MemoryJobStore struct {
	mu   sync.RWMutex
	jobs map[string]Report
}

func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{jobs: make(map[string]Report)}
}

func (s *MemoryJobStore) Create(_ context.Context, report Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[report.ID] = cloneReport(report)
	return nil
}

func (s *MemoryJobStore) Update(_ context.Context, report Report, _ *FileResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[report.ID]; !exists {
		return ErrJobNotFound
	}
	s.jobs[report.ID] = cloneReport(report)
	return nil
}

func (s *MemoryJobStore) Get(_ context.Context, id string) (Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, exists := s.jobs[id]
	if !exists {
		return Report{}, ErrJobNotFound
	}
	return cloneReport(report), nil
}

func cloneReport(report Report) Report {
	report.Files = append([]FileResult(nil), report.Files...)
	return report
}
