package reranking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

type HTTP struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewHTTP(baseURL, apiKey, model string, timeout time.Duration) (*HTTP, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	model = strings.TrimSpace(model)
	if baseURL == "" || model == "" {
		return nil, fmt.Errorf("rerank base URL and model are required")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HTTP{baseURL: baseURL, apiKey: strings.TrimSpace(apiKey), model: model, client: &http.Client{Timeout: timeout}}, nil
}

func (p *HTTP) Name() string {
	return "http-rerank:" + p.model
}

func (p *HTTP) Rerank(ctx context.Context, query string, candidates []Candidate) (Output, error) {
	documents := make([]map[string]string, len(candidates))
	for index, candidate := range candidates {
		documents[index] = map[string]string{"title": candidate.Title, "text": candidate.Content}
	}
	payload, err := json.Marshal(map[string]any{
		"model": p.model, "query": query, "documents": documents, "top_n": len(documents),
	})
	if err != nil {
		return Output{}, fmt.Errorf("encode rerank request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/rerank", bytes.NewReader(payload))
	if err != nil {
		return Output{}, fmt.Errorf("create rerank request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return Output{}, fmt.Errorf("call rerank service: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return Output{}, fmt.Errorf("rerank service returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return Output{}, fmt.Errorf("decode rerank response: %w", err)
	}
	if len(result.Results) != len(candidates) {
		return Output{}, fmt.Errorf("rerank service returned %d results for %d candidates", len(result.Results), len(candidates))
	}
	scores := make([]float64, len(candidates))
	seen := make([]bool, len(candidates))
	for _, item := range result.Results {
		if item.Index < 0 || item.Index >= len(candidates) || seen[item.Index] || math.IsNaN(item.RelevanceScore) || math.IsInf(item.RelevanceScore, 0) {
			return Output{}, fmt.Errorf("rerank service returned an invalid result")
		}
		seen[item.Index] = true
		scores[item.Index] = item.RelevanceScore
	}
	return Output{Scores: scores, Provider: p.Name()}, nil
}
