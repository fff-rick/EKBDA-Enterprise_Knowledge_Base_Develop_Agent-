package reranking

import (
	"context"
	"errors"
	"testing"
)

func TestLocalRewardsExactAndRetrievalSignals(t *testing.T) {
	provider := NewLocal()
	output, err := provider.Rerank(context.Background(), "start service", []Candidate{
		{Title: "Unrelated", Content: "other text", FusionScore: 0.01, VectorScore: 0.2},
		{Title: "Start service", Content: "run the command", FusionScore: 0.02, KeywordScore: 4, VectorScore: 0.8},
	})
	if err != nil {
		t.Fatalf("rerank: %v", err)
	}
	if len(output.Scores) != 2 || output.Scores[1] <= output.Scores[0] || output.Provider != "local-weighted-v1" {
		t.Fatalf("unexpected local rerank output: %#v", output)
	}
}

func TestFallbackUsesLocalWhenPrimaryFails(t *testing.T) {
	provider := NewFallback(errorProvider{}, NewLocal())
	output, err := provider.Rerank(context.Background(), "query", []Candidate{{Title: "query", FusionScore: 1}})
	if err != nil {
		t.Fatalf("fallback rerank: %v", err)
	}
	if output.Provider != "local-weighted-v1" || len(output.Scores) != 1 {
		t.Fatalf("unexpected fallback output: %#v", output)
	}
}

type errorProvider struct{}

func (errorProvider) Name() string { return "error" }
func (errorProvider) Rerank(context.Context, string, []Candidate) (Output, error) {
	return Output{}, errors.New("unavailable")
}
