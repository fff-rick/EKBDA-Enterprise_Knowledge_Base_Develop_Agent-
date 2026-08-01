package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAICompatibleEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 1, "embedding": []float32{0, 1}},
				{"index": 0, "embedding": []float32{1, 0}},
			},
		})
	}))
	defer server.Close()
	provider, err := NewOpenAICompatible(server.URL+"/v1", "secret", "embedding-model", time.Second)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	vectors, err := provider.Embed(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][1] != 1 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}
