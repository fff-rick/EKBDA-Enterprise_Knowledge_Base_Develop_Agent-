package reranking

import "context"

type Candidate struct {
	Title        string
	Content      string
	KeywordScore int
	VectorScore  float64
	FusionScore  float64
}

type Output struct {
	Scores   []float64
	Provider string
}

type Provider interface {
	Rerank(ctx context.Context, query string, candidates []Candidate) (Output, error)
	Name() string
}
