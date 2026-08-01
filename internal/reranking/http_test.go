package reranking

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPReranker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"index": 1, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.1},
			},
		})
	}))
	defer server.Close()
	provider, err := NewHTTP(server.URL+"/v1", "secret", "rerank-model", time.Second)
	if err != nil {
		t.Fatalf("create HTTP reranker: %v", err)
	}
	output, err := provider.Rerank(context.Background(), "query", []Candidate{{Content: "first"}, {Content: "second"}})
	if err != nil {
		t.Fatalf("rerank: %v", err)
	}
	if output.Scores[0] != 0.1 || output.Scores[1] != 0.9 || output.Provider != "http-rerank:rerank-model" {
		t.Fatalf("unexpected HTTP rerank output: %#v", output)
	}
}
