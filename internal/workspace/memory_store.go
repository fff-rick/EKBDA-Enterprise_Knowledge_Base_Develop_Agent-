package workspace

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type MemoryStore struct {
	mu        sync.RWMutex
	snapshots map[string]Snapshot
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{snapshots: make(map[string]Snapshot)}
}

func (s *MemoryStore) Save(_ context.Context, snapshot Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[snapshot.ID] = cloneSnapshot(snapshot)
	return nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, exists := s.snapshots[id]
	if !exists {
		return Snapshot{}, ErrSnapshotNotFound
	}
	return cloneSnapshot(snapshot), nil
}

func (s *MemoryStore) List(_ context.Context, project string, limit int) ([]Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project = strings.ToLower(strings.TrimSpace(project))
	result := make([]Snapshot, 0)
	for _, snapshot := range s.snapshots {
		if project == "" || snapshot.Project == project {
			result = append(result, cloneSnapshot(snapshot))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Changes = append([]Change(nil), snapshot.Changes...)
	return snapshot
}
