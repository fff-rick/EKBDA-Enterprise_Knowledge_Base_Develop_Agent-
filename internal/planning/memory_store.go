package planning

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
	if _, exists := s.sessions[session.ID]; exists {
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
		if project == "" || session.Project == project {
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

func cloneSession(session Session) Session {
	session.AcceptanceCriteria = append([]string(nil), session.AcceptanceCriteria...)
	session.Constraints = append([]string(nil), session.Constraints...)
	session.OutOfScope = append([]string(nil), session.OutOfScope...)
	session.Questions = append([]Question(nil), session.Questions...)
	session.Answers = append([]ClarificationAnswer(nil), session.Answers...)
	session.Context.Knowledge = append([]KnowledgeReference(nil), session.Context.Knowledge...)
	for index := range session.Context.Knowledge {
		if session.Context.Knowledge[index].Citation != nil {
			citation := *session.Context.Knowledge[index].Citation
			session.Context.Knowledge[index].Citation = &citation
		}
	}
	session.Context.Standards = append([]StandardReference(nil), session.Context.Standards...)
	for index := range session.Context.Standards {
		session.Context.Standards[index].Rules = append([]RuleReference(nil), session.Context.Standards[index].Rules...)
	}
	session.Context.Repository.ChangedPaths = append([]string(nil), session.Context.Repository.ChangedPaths...)
	if session.Plan != nil {
		plan := *session.Plan
		plan.Assumptions = append([]string(nil), plan.Assumptions...)
		plan.OutOfScope = append([]string(nil), plan.OutOfScope...)
		plan.Risks = append([]Risk(nil), plan.Risks...)
		plan.Steps = append([]PlanStep(nil), plan.Steps...)
		for index := range plan.Steps {
			plan.Steps[index].Deliverables = append([]string(nil), plan.Steps[index].Deliverables...)
			plan.Steps[index].Verification = append([]string(nil), plan.Steps[index].Verification...)
			plan.Steps[index].KnowledgeReferences = append([]string(nil), plan.Steps[index].KnowledgeReferences...)
			plan.Steps[index].StandardReferences = append([]string(nil), plan.Steps[index].StandardReferences...)
		}
		session.Plan = &plan
	}
	if session.RoleReview != nil {
		cycle := *session.RoleReview
		cycle.Context.Knowledge = append([]KnowledgeReference(nil), cycle.Context.Knowledge...)
		for index := range cycle.Context.Knowledge {
			if cycle.Context.Knowledge[index].Citation != nil {
				citation := *cycle.Context.Knowledge[index].Citation
				cycle.Context.Knowledge[index].Citation = &citation
			}
		}
		cycle.Context.Standards = append([]StandardReference(nil), cycle.Context.Standards...)
		for index := range cycle.Context.Standards {
			cycle.Context.Standards[index].Rules = append([]RuleReference(nil), cycle.Context.Standards[index].Rules...)
		}
		cycle.Context.Repository.ChangedPaths = append([]string(nil), cycle.Context.Repository.ChangedPaths...)
		cycle.Reviews = append([]RoleReview(nil), cycle.Reviews...)
		for index := range cycle.Reviews {
			cycle.Reviews[index].Findings = append([]ReviewFinding(nil), cycle.Reviews[index].Findings...)
			cycle.Reviews[index].OpenQuestions = append([]string(nil), cycle.Reviews[index].OpenQuestions...)
			cycle.Reviews[index].KnowledgeReferences = append([]string(nil), cycle.Reviews[index].KnowledgeReferences...)
			cycle.Reviews[index].StandardReferences = append([]string(nil), cycle.Reviews[index].StandardReferences...)
		}
		cycle.Coordination.Consensus = append([]string(nil), cycle.Coordination.Consensus...)
		cycle.Coordination.DecisionItems = append([]DecisionItem(nil), cycle.Coordination.DecisionItems...)
		for index := range cycle.Coordination.DecisionItems {
			cycle.Coordination.DecisionItems[index].Options = append([]string(nil), cycle.Coordination.DecisionItems[index].Options...)
			cycle.Coordination.DecisionItems[index].SourceRoles = append([]string(nil), cycle.Coordination.DecisionItems[index].SourceRoles...)
			if cycle.Coordination.DecisionItems[index].ResolvedAt != nil {
				resolvedAt := *cycle.Coordination.DecisionItems[index].ResolvedAt
				cycle.Coordination.DecisionItems[index].ResolvedAt = &resolvedAt
			}
		}
		session.RoleReview = &cycle
	}
	return session
}
