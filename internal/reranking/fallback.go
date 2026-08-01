package reranking

import "context"

type Fallback struct {
	primary  Provider
	fallback Provider
}

func NewFallback(primary, fallback Provider) *Fallback {
	return &Fallback{primary: primary, fallback: fallback}
}

func (p *Fallback) Name() string {
	return p.primary.Name() + "->" + p.fallback.Name()
}

func (p *Fallback) Rerank(ctx context.Context, query string, candidates []Candidate) (Output, error) {
	result, err := p.primary.Rerank(ctx, query, candidates)
	if err == nil {
		return result, nil
	}
	return p.fallback.Rerank(ctx, query, candidates)
}
