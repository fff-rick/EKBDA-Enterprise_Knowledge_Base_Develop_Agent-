package initiative

import (
	"context"
	"sort"
	"sync"
)

type MemoryStore struct {
	mu       sync.RWMutex
	packages map[string]Package
	reviews  map[string][]ArtifactReview
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{packages: make(map[string]Package), reviews: make(map[string][]ArtifactReview)}
}

func (s *MemoryStore) CreateReview(_ context.Context, review ArtifactReview) (ArtifactReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found := s.packages[review.PackageID]; !found {
		return ArtifactReview{}, ErrPackageNotFound
	}
	review.Sequence = 1
	for _, existing := range s.reviews[review.PackageID] {
		if existing.ArtifactType == review.ArtifactType && existing.Sequence >= review.Sequence {
			review.Sequence = existing.Sequence + 1
		}
	}
	s.reviews[review.PackageID] = append(s.reviews[review.PackageID], review)
	return review, nil
}

func (s *MemoryStore) ListReviews(_ context.Context, packageID, artifactType string, limit int) ([]ArtifactReview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, found := s.packages[packageID]; !found {
		return nil, ErrPackageNotFound
	}
	result := make([]ArtifactReview, 0)
	for _, review := range s.reviews[packageID] {
		if artifactType == "" || review.ArtifactType == artifactType {
			result = append(result, review)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ArtifactType != result[j].ArtifactType {
			return result[i].ArtifactType < result[j].ArtifactType
		}
		return result[i].Sequence > result[j].Sequence
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *MemoryStore) Create(_ context.Context, projectPackage Package) (Package, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version := 1
	for _, existing := range s.packages {
		if existing.Project == projectPackage.Project && existing.Name == projectPackage.Name && existing.Version >= version {
			version = existing.Version + 1
		}
	}
	projectPackage.Version = version
	s.packages[projectPackage.ID] = clonePackage(projectPackage)
	return clonePackage(projectPackage), nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Package, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	projectPackage, found := s.packages[id]
	if !found {
		return Package{}, ErrPackageNotFound
	}
	return clonePackage(projectPackage), nil
}

func (s *MemoryStore) List(_ context.Context, project, name string, limit int) ([]Package, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Package, 0)
	for _, projectPackage := range s.packages {
		if projectPackage.Project == project && (name == "" || projectPackage.Name == name) {
			result = append(result, clonePackage(projectPackage))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Version > result[j].Version
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func clonePackage(projectPackage Package) Package {
	projectPackage.Artifacts = append([]Artifact(nil), projectPackage.Artifacts...)
	for artifactIndex := range projectPackage.Artifacts {
		artifact := &projectPackage.Artifacts[artifactIndex]
		artifact.Sections = append([]Section(nil), artifact.Sections...)
		artifact.References = append([]Reference(nil), artifact.References...)
		for sectionIndex := range artifact.Sections {
			artifact.Sections[sectionIndex].Items = append([]string(nil), artifact.Sections[sectionIndex].Items...)
		}
	}
	projectPackage.Traceability = append([]TraceRecord(nil), projectPackage.Traceability...)
	for index := range projectPackage.Traceability {
		trace := &projectPackage.Traceability[index]
		trace.ArchitectureSections = append([]string(nil), trace.ArchitectureSections...)
		trace.APISections = append([]string(nil), trace.APISections...)
		trace.TestSections = append([]string(nil), trace.TestSections...)
		trace.DeploymentSections = append([]string(nil), trace.DeploymentSections...)
		trace.Gaps = append([]string(nil), trace.Gaps...)
	}
	if projectPackage.Source.PlanApprovedAt != nil {
		approvedAt := *projectPackage.Source.PlanApprovedAt
		projectPackage.Source.PlanApprovedAt = &approvedAt
	}
	return projectPackage
}
