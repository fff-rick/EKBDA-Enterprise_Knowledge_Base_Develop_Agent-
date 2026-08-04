package release

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type providerReceipt struct{ releaseID, hash string }

type MemoryStore struct {
	mu       sync.RWMutex
	requests map[string]Request
	events   map[string][]Event
	receipts map[string]providerReceipt
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{requests: map[string]Request{}, events: map[string][]Event{}, receipts: map[string]providerReceipt{}}
}

func (s *MemoryStore) Create(_ context.Context, request Request, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.requests[request.ID]; ok {
		return ErrRevisionConflict
	}
	s.requests[request.ID] = cloneRequest(request)
	s.events[request.ID] = []Event{event}
	return nil
}

func (s *MemoryStore) Update(_ context.Context, request Request, expected int, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.requests[request.ID]
	if !ok {
		return ErrNotFound
	}
	if current.Revision != expected {
		return ErrRevisionConflict
	}
	s.requests[request.ID] = cloneRequest(request)
	s.events[request.ID] = append(s.events[request.ID], event)
	return nil
}

func (s *MemoryStore) ApplyProviderEvent(_ context.Context, request Request, expected int, event Event, providerEventID, payloadHash string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt, ok := s.receipts[providerEventID]; ok {
		if receipt.releaseID != request.ID || receipt.hash != payloadHash {
			return false, ErrProviderConflict
		}
		return false, nil
	}
	current, ok := s.requests[request.ID]
	if !ok {
		return false, ErrNotFound
	}
	if current.Revision != expected {
		return false, ErrRevisionConflict
	}
	s.receipts[providerEventID] = providerReceipt{request.ID, payloadHash}
	s.requests[request.ID] = cloneRequest(request)
	s.events[request.ID] = append(s.events[request.ID], event)
	return true, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.requests[id]
	if !ok {
		return Request{}, ErrNotFound
	}
	return cloneRequest(value), nil
}

func (s *MemoryStore) List(_ context.Context, project string, limit int) ([]Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project = strings.ToLower(strings.TrimSpace(project))
	result := make([]Request, 0)
	for _, value := range s.requests {
		if value.Project == project {
			result = append(result, cloneRequest(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListEvents(_ context.Context, id string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.requests[id]; !ok {
		return nil, ErrNotFound
	}
	return append([]Event(nil), s.events[id]...), nil
}

func cloneRequest(value Request) Request {
	value.RequiredChecks = append([]string(nil), value.RequiredChecks...)
	value.Checks = append([]CheckEvidence(nil), value.Checks...)
	if value.Artifact != nil {
		artifact := *value.Artifact
		value.Artifact = &artifact
	}
	if value.Source != nil {
		source := *value.Source
		value.Source = &source
	}
	if value.ApprovedAt != nil {
		timestamp := *value.ApprovedAt
		value.ApprovedAt = &timestamp
	}
	if value.CompletedAt != nil {
		timestamp := *value.CompletedAt
		value.CompletedAt = &timestamp
	}
	return value
}
