package access

import (
	"context"
	"sort"
	"sync"
)

type MemoryStore struct {
	mu       sync.RWMutex
	policies map[string]Policy
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{policies: make(map[string]Policy)}
}

func (s *MemoryStore) CreatePolicy(_ context.Context, policy Policy) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	policy.Version = 1
	for _, existing := range s.policies {
		if existing.Project == policy.Project && existing.Version >= policy.Version {
			policy.Version = existing.Version + 1
		}
	}
	s.policies[policy.ID] = clonePolicy(policy)
	return clonePolicy(policy), nil
}

func (s *MemoryStore) GetLatest(_ context.Context, project string) (Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest Policy
	found := false
	for _, policy := range s.policies {
		if policy.Project == project && (!found || policy.Version > latest.Version) {
			latest = policy
			found = true
		}
	}
	if !found {
		return Policy{}, ErrPolicyNotFound
	}
	return clonePolicy(latest), nil
}

func (s *MemoryStore) ListPolicies(_ context.Context, project string, limit int) ([]Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Policy, 0)
	for _, policy := range s.policies {
		if policy.Project == project {
			result = append(result, clonePolicy(policy))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version > result[j].Version })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func clonePolicy(policy Policy) Policy {
	policy.Users = append([]string(nil), policy.Users...)
	policy.Roles = append([]string(nil), policy.Roles...)
	policy.Repositories = append([]string(nil), policy.Repositories...)
	return policy
}
