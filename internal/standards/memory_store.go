package standards

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type MemoryStore struct {
	mu       sync.RWMutex
	packages map[string]Package
	reports  map[string]ValidationReport
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{packages: make(map[string]Package), reports: make(map[string]ValidationReport)}
}

func (s *MemoryStore) CreatePackage(_ context.Context, standard Package) (Package, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version := 1
	for _, existing := range s.packages {
		if existing.Name == standard.Name && existing.Scope == standard.Scope && existing.Selector == standard.Selector && existing.Version >= version {
			version = existing.Version + 1
		}
	}
	standard.Version = version
	s.packages[standard.ID] = clonePackage(standard)
	return clonePackage(standard), nil
}

func (s *MemoryStore) GetPackage(_ context.Context, id string) (Package, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	standard, exists := s.packages[id]
	if !exists {
		return Package{}, ErrPackageNotFound
	}
	return clonePackage(standard), nil
}

func (s *MemoryStore) ListPackages(_ context.Context, name, scope, selector string, limit int) ([]Package, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Package, 0)
	for _, standard := range s.packages {
		if (name == "" || standard.Name == name) && (scope == "" || standard.Scope == scope) && (selector == "" || standard.Selector == selector) {
			result = append(result, clonePackage(standard))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		if result[i].Scope != result[j].Scope {
			return scopePriority(result[i].Scope) < scopePriority(result[j].Scope)
		}
		if result[i].Selector != result[j].Selector {
			return result[i].Selector < result[j].Selector
		}
		return result[i].Version > result[j].Version
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) ListApplicable(_ context.Context, project, technology string) ([]Package, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	latest := make(map[string]Package)
	for _, standard := range s.packages {
		applicable := standard.Scope == ScopeCommon ||
			(standard.Scope == ScopeTechnology && standard.Selector == technology) ||
			(standard.Scope == ScopeProject && standard.Selector == project)
		if !applicable {
			continue
		}
		key := standard.Scope + "\x00" + standard.Selector + "\x00" + standard.Name
		if existing, found := latest[key]; !found || standard.Version > existing.Version {
			latest[key] = standard
		}
	}
	result := make([]Package, 0, len(latest))
	for _, standard := range latest {
		result = append(result, clonePackage(standard))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Scope != result[j].Scope {
			return scopePriority(result[i].Scope) < scopePriority(result[j].Scope)
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (s *MemoryStore) SaveReport(_ context.Context, report ValidationReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[report.ID] = cloneReport(report)
	return nil
}

func (s *MemoryStore) GetReport(_ context.Context, id string) (ValidationReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, exists := s.reports[id]
	if !exists {
		return ValidationReport{}, ErrReportNotFound
	}
	return cloneReport(report), nil
}

func (s *MemoryStore) ListReports(_ context.Context, project string, limit int) ([]ValidationReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project = strings.TrimSpace(strings.ToLower(project))
	result := make([]ValidationReport, 0)
	for _, report := range s.reports {
		if project == "" || report.Project == project {
			result = append(result, cloneReport(report))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func clonePackage(standard Package) Package {
	standard.Rules = cloneRules(standard.Rules)
	return standard
}

func cloneRules(rules []Rule) []Rule {
	result := append([]Rule(nil), rules...)
	for index := range result {
		if result[index].Check != nil {
			check := *result[index].Check
			result[index].Check = &check
		}
	}
	return result
}

func cloneReport(report ValidationReport) ValidationReport {
	report.Packages = append([]PackageReference(nil), report.Packages...)
	report.Violations = append([]Violation(nil), report.Violations...)
	return report
}

func scopePriority(scope string) int {
	switch scope {
	case ScopeCommon:
		return 1
	case ScopeTechnology:
		return 2
	case ScopeProject:
		return 3
	default:
		return 4
	}
}
