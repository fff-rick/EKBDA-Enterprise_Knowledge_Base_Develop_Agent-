package reranking

import (
	"context"
	"strings"
)

type Local struct{}

func NewLocal() *Local {
	return &Local{}
}

func (*Local) Name() string {
	return "local-weighted-v1"
}

func (p *Local) Rerank(ctx context.Context, query string, candidates []Candidate) (Output, error) {
	maxKeyword := 1
	maxFusion := 0.0
	for _, candidate := range candidates {
		if candidate.KeywordScore > maxKeyword {
			maxKeyword = candidate.KeywordScore
		}
		if candidate.FusionScore > maxFusion {
			maxFusion = candidate.FusionScore
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	scores := make([]float64, len(candidates))
	for index, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		fusion := 0.0
		if maxFusion > 0 {
			fusion = candidate.FusionScore / maxFusion
		}
		vector := candidate.VectorScore
		if vector < 0 {
			vector = 0
		}
		exact := 0.0
		if query != "" && strings.Contains(strings.ToLower(candidate.Title+"\n"+candidate.Content), query) {
			exact = 1
		}
		scores[index] = 0.45*fusion + 0.25*(float64(candidate.KeywordScore)/float64(maxKeyword)) + 0.25*vector + 0.05*exact
	}
	return Output{Scores: scores, Provider: p.Name()}, nil
}
