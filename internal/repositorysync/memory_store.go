package repositorysync

import (
	"context"
	"sort"
	"strings"
	"sync"

	"ekbda/internal/workspace"
)

type MemoryStore struct {
	mu      sync.RWMutex
	reports map[string]Report
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{reports: make(map[string]Report)}
}

func (s *MemoryStore) Save(_ context.Context, report Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[report.ID] = cloneReport(report)
	return nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, found := s.reports[id]
	if !found {
		return Report{}, ErrReportNotFound
	}
	return cloneReport(report), nil
}

func (s *MemoryStore) List(_ context.Context, project string, limit int) ([]Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project = strings.ToLower(strings.TrimSpace(project))
	result := make([]Report, 0)
	for _, report := range s.reports {
		if project == "" || report.Project == project {
			result = append(result, cloneReport(report))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.After(result[j].StartedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) LatestCompleted(_ context.Context, project, repository string) (Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest Report
	found := false
	for _, report := range s.reports {
		if report.Project == project && report.Repository == repository && report.Status == StatusCompleted && (!found || report.CompletedAt.After(latest.CompletedAt)) {
			latest = report
			found = true
		}
	}
	if !found {
		return Report{}, ErrReportNotFound
	}
	return cloneReport(latest), nil
}

func cloneReport(report Report) Report {
	report.AllowedRoles = append([]string(nil), report.AllowedRoles...)
	report.CommitChanges = append([]workspace.CommitChange(nil), report.CommitChanges...)
	report.Files = append([]FileResult(nil), report.Files...)
	return report
}
