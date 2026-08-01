package planning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ekbda/internal/agenttask"
)

func TestOpenAICompatiblePlanningProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected planning request: %s %#v", r.URL.Path, r.Header)
		}
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Fatalf("decode planning request: %#v err=%v", request, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"questions\":[{\"id\":\"acceptance\",\"question\":\"验收标准？\",\"reason\":\"需要验证\"}]}"}}]}`))
	}))
	defer server.Close()
	provider, err := NewOpenAICompatibleProvider(server.URL+"/v1", "secret", "planner-model", time.Second)
	if err != nil {
		t.Fatalf("create planning provider: %v", err)
	}
	questions, err := provider.Clarify(context.Background(), Session{Requirement: "requirement"})
	if err != nil || len(questions) != 1 || questions[0].ID != "acceptance" {
		t.Fatalf("unexpected planning response: %#v err=%v", questions, err)
	}
}

func TestOpenAICompatibleRoleReviewProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Fatalf("decode role review request: %#v err=%v", request, err)
		}
		var input struct {
			Operation string `json:"operation"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &input); err != nil {
			t.Fatalf("decode role review operation: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch input.Operation {
		case "role_review":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"review\":{\"summary\":\"backend review\",\"recommendation\":\"approve\",\"findings\":[],\"open_questions\":[],\"knowledge_references\":[],\"standard_references\":[]}}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
		case "coordinate":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"coordination\":{\"summary\":\"coordinated\",\"consensus\":[\"approved\"],\"decision_items\":[]}}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
		default:
			t.Fatalf("unexpected operation: %s", input.Operation)
		}
	}))
	defer server.Close()
	provider, err := NewOpenAICompatibleProvider(server.URL+"/v1", "secret", "planner-model", time.Second)
	if err != nil {
		t.Fatalf("create planning provider: %v", err)
	}
	collector := &agenttask.UsageCollector{}
	ctx := agenttask.WithUsageCollector(context.Background(), collector)
	review, err := provider.ReviewRole(ctx, RoleBackendEngineer, Session{})
	if err != nil || review.Summary != "backend review" {
		t.Fatalf("unexpected role review: %#v err=%v", review, err)
	}
	coordination, err := provider.Coordinate(ctx, Session{}, []RoleReview{review})
	if err != nil || coordination.Summary != "coordinated" {
		t.Fatalf("unexpected coordination: %#v err=%v", coordination, err)
	}
	if usage := collector.Snapshot(agenttask.Pricing{}); usage.TotalTokens != 30 {
		t.Fatalf("unexpected role-review usage: %#v", usage)
	}
}
