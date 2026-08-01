package development

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
	events   map[string][]Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]Session), events: make(map[string][]Event)}
}

func (s *MemoryStore) Create(_ context.Context, session Session, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found := s.sessions[session.ID]; found {
		return ErrRevisionConflict
	}
	s.sessions[session.ID] = cloneSession(session)
	s.events[session.ID] = []Event{event}
	return nil
}

func (s *MemoryStore) Update(_ context.Context, session Session, expectedRevision int, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, found := s.sessions[session.ID]
	if !found {
		return ErrSessionNotFound
	}
	if existing.Revision != expectedRevision {
		return ErrRevisionConflict
	}
	s.sessions[session.ID] = cloneSession(session)
	s.events[session.ID] = append(s.events[session.ID], event)
	return nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, found := s.sessions[id]
	if !found {
		return Session{}, ErrSessionNotFound
	}
	return cloneSession(session), nil
}

func (s *MemoryStore) List(_ context.Context, project string, limit int) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project = strings.ToLower(strings.TrimSpace(project))
	result := make([]Session, 0)
	for _, session := range s.sessions {
		if session.Project == project {
			result = append(result, cloneSession(session))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListEvents(_ context.Context, sessionID string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, found := s.sessions[sessionID]; !found {
		return nil, ErrSessionNotFound
	}
	return append([]Event(nil), s.events[sessionID]...), nil
}

func (s *MemoryStore) ListExecuting(_ context.Context, limit int) ([]Session, error) {
	return s.listStatus(StatusExecuting, limit), nil
}

func (s *MemoryStore) ListDelivering(_ context.Context, limit int) ([]Session, error) {
	return s.listStatus(StatusDelivering, limit), nil
}

func (s *MemoryStore) listStatus(status string, limit int) []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Session, 0)
	for _, session := range s.sessions {
		if session.Status == status {
			result = append(result, cloneSession(session))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.Before(result[j].UpdatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func cloneSession(session Session) Session {
	session.AllowedPaths = append([]string(nil), session.AllowedPaths...)
	session.AllowedCommands = append([]string(nil), session.AllowedCommands...)
	if session.Proposal != nil {
		proposal := *session.Proposal
		proposal.Files = append([]FileChange(nil), proposal.Files...)
		proposal.Commands = cloneCommands(proposal.Commands)
		session.Proposal = &proposal
	}
	if session.ReviewedAt != nil {
		value := *session.ReviewedAt
		session.ReviewedAt = &value
	}
	if session.Execution != nil {
		execution := *session.Execution
		execution.Commands = append([]CommandEvidence(nil), execution.Commands...)
		if execution.SecretScan != nil {
			scan := *execution.SecretScan
			execution.SecretScan = &scan
		}
		session.Execution = &execution
	}
	if session.Delivery != nil {
		delivery := *session.Delivery
		if delivery.SecretScan != nil {
			scan := *delivery.SecretScan
			delivery.SecretScan = &scan
		}
		session.Delivery = &delivery
	}
	return session
}
