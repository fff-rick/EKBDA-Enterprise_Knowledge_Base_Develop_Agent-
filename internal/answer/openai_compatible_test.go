package answer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAICompatibleGroundedAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{
					"content": `{"answer":"启动服务。","refused":false,"refusal_reason":"","citation_ids":["E1"]}`,
				},
			}},
			"usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 7, "total_tokens": 19},
		})
	}))
	defer server.Close()
	provider, err := NewOpenAICompatible(server.URL+"/v1", "secret", "chat-model", time.Second)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	draft, err := provider.Generate(context.Background(), "如何启动", []Evidence{{ID: "E1", Snippet: "运行服务"}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if draft.Answer != "启动服务。" || len(draft.CitationIDs) != 1 || draft.CitationIDs[0] != "E1" {
		t.Fatalf("unexpected draft: %#v", draft)
	}
	if draft.Usage.TotalTokens != 19 {
		t.Fatalf("unexpected token usage: %#v", draft.Usage)
	}
}
