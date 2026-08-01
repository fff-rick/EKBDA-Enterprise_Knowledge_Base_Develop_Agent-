package answer

import (
	"context"
	"strings"
	"sync"
	"time"
)

type MemoryTraceStore struct {
	mu     sync.RWMutex
	traces map[string]Trace
}

func NewMemoryTraceStore() *MemoryTraceStore {
	return &MemoryTraceStore{traces: make(map[string]Trace)}
}

func (s *MemoryTraceStore) Save(_ context.Context, trace Trace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traces[trace.ID] = trace
	return nil
}

func (s *MemoryTraceStore) Get(_ context.Context, id string) (Trace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	trace, exists := s.traces[id]
	if !exists {
		return Trace{}, ErrTraceNotFound
	}
	return trace, nil
}

func (s *MemoryTraceStore) Metrics(_ context.Context, project string) (Metrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project = strings.TrimSpace(project)
	metrics := Metrics{Project: project, ByProvider: make(map[string]int)}
	var totalDuration int64
	for _, trace := range s.traces {
		if project != "" && trace.Project != project {
			continue
		}
		metrics.Total++
		if trace.Status == "succeeded" {
			metrics.Succeeded++
		} else {
			metrics.Errors++
		}
		if trace.Refused {
			metrics.Refused++
		}
		totalDuration += trace.DurationMS
		metrics.TotalTokens += int64(trace.TotalTokens)
		metrics.TotalCostUSD += trace.TotalCostUSD
		metrics.ByProvider[trace.Provider]++
	}
	if metrics.Total > 0 {
		metrics.AverageDuration = float64(totalDuration) / float64(metrics.Total)
	}
	return metrics, nil
}

func (s *MemoryTraceStore) DeleteBefore(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	for id, trace := range s.traces {
		if trace.CreatedAt.Before(before) {
			delete(s.traces, id)
			deleted++
		}
	}
	return deleted, nil
}
