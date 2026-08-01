package knowledge

import "context"

type Repository interface {
	Save(ctx context.Context, document Document) error
	Update(ctx context.Context, document Document) error
	FindBySource(ctx context.Context, project, sourceURI string) (Document, bool, error)
	List(ctx context.Context) ([]Document, error)
	ListVersions(ctx context.Context, documentID string) ([]DocumentVersion, error)
}

type candidateSearchInput struct {
	Query             string
	Project           string
	Roles             []string
	Tokens            []string
	QueryVector       []float32
	EmbeddingProvider string
	Limit             int
}

type candidateRepository interface {
	SearchCandidates(ctx context.Context, input candidateSearchInput) ([]searchCandidate, error)
}
