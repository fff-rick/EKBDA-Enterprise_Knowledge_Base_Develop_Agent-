package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type OpenAICompatible struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAICompatible(baseURL, apiKey, model string, timeout time.Duration) (*OpenAICompatible, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	model = strings.TrimSpace(model)
	if baseURL == "" || model == "" {
		return nil, fmt.Errorf("embedding base URL and model are required")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &OpenAICompatible{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (p *OpenAICompatible) Name() string {
	return "openai-compatible:" + p.model
}

func (p *OpenAICompatible) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	payload, err := json.Marshal(map[string]any{
		"model":           p.model,
		"input":           texts,
		"encoding_format": "float",
	})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call embedding service: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("embedding service returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("embedding service returned %d vectors for %d texts", len(result.Data), len(texts))
	}
	sort.Slice(result.Data, func(i, j int) bool { return result.Data[i].Index < result.Data[j].Index })
	vectors := make([][]float32, len(result.Data))
	for index, item := range result.Data {
		if item.Index != index {
			return nil, fmt.Errorf("embedding service returned invalid index %d at position %d", item.Index, index)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("embedding service returned an empty vector at index %d", index)
		}
		vectors[index] = item.Embedding
	}
	return vectors, nil
}
