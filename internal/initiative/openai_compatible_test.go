package initiative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ekbda/internal/agenttask"
	"ekbda/internal/planning"
)

func TestOpenAICompatibleProjectPackageProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected project package request: %s %#v", r.URL.Path, r.Header)
		}
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Fatalf("decode project package request: %#v err=%v", request, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"artifacts\":[{\"type\":\"prd\",\"title\":\"PRD\",\"summary\":\"summary\",\"sections\":[{\"name\":\"goal\",\"items\":[\"item\"]}],\"references\":[]}],\"traceability\":[{\"requirement_id\":\"REQ-001\",\"requirement\":\"item\"}]}"}}],"usage":{"prompt_tokens":240,"completion_tokens":60}}`))
	}))
	defer server.Close()
	provider, err := NewOpenAICompatibleProvider(server.URL+"/v1", "secret", "package-model", time.Second)
	if err != nil {
		t.Fatalf("create project package provider: %v", err)
	}
	collector := &agenttask.UsageCollector{}
	ctx := agenttask.WithUsageCollector(context.Background(), collector)
	output, err := provider.Build(ctx, planning.Session{ID: "session"})
	if err != nil || len(output.Artifacts) != 1 || output.Artifacts[0].Type != ArtifactPRD || len(output.Traceability) != 1 {
		t.Fatalf("unexpected project package response: %#v err=%v", output, err)
	}
	if usage := collector.Snapshot(agenttask.Pricing{}); usage.TotalTokens != 300 {
		t.Fatalf("unexpected project package usage: %#v", usage)
	}
}
