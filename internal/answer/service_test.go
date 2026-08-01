package answer

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"ekbda/internal/embedding"
	"ekbda/internal/knowledge"
)

type fixedProvider struct {
	draft Draft
	err   error
}

func (p fixedProvider) Generate(_ context.Context, _ string, _ []Evidence) (Draft, error) {
	return p.draft, p.err
}

func (p fixedProvider) Name() string { return "fixed" }

func TestAskReturnsGroundedLocalAnswer(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title:     "订单服务启动说明",
		Content:   "运行 go run ./cmd/server 启动订单服务。",
		SourceURI: "git://order/README.md",
		Project:   "order",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	service := NewService(knowledgeService, NewLocalExtractive(), NewMemoryTraceStore())
	response, err := service.Ask(context.Background(), Input{Query: "如何启动订单服务", Project: "order"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if response.Refused || len(response.Citations) == 0 || response.Citations[0].Citation.SourceURI != "git://order/README.md" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Citations[0].FusionScore <= 0 || response.Citations[0].RerankScore <= 0 || response.Citations[0].Reranker != "local-weighted-v1" {
		t.Fatalf("citation is missing retrieval scores: %#v", response.Citations[0])
	}
}

func TestAskRefusesWhenEvidenceIsUnavailable(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	service := NewService(knowledgeService, NewLocalExtractive(), NewMemoryTraceStore())
	response, err := service.Ask(context.Background(), Input{Query: "生产数据库密码是什么", Project: "order"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !response.Refused || response.RefusalReason != "insufficient_evidence" || len(response.Citations) != 0 {
		t.Fatalf("unexpected refusal: %#v", response)
	}
}

func TestAskRejectsProviderCitationOutsideEvidence(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title:     "部署说明",
		Content:   "使用流水线部署。",
		SourceURI: "wiki://deploy",
		Project:   "platform",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	service := NewService(knowledgeService, fixedProvider{draft: Draft{
		Answer:      "未经证据支持的回答",
		CitationIDs: []string{"E999"},
	}}, NewMemoryTraceStore())
	response, err := service.Ask(context.Background(), Input{Query: "如何部署", Project: "platform"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !response.Refused || response.RefusalReason != "invalid_citation" {
		t.Fatalf("expected invalid citation refusal: %#v", response)
	}
}

func TestAskCannotUseRestrictedEvidenceWithoutRole(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title:          "财务审批规则",
		Content:        "退款必须经过财务审批。",
		SourceURI:      "wiki://finance/refund",
		Project:        "order",
		Classification: knowledge.ClassificationRestricted,
		AllowedRoles:   []string{"finance"},
	})
	if err != nil {
		t.Fatalf("create restricted knowledge: %v", err)
	}
	service := NewService(knowledgeService, NewLocalExtractive(), NewMemoryTraceStore())
	response, err := service.Ask(context.Background(), Input{
		Query:   "退款需要谁审批",
		Project: "order",
		Roles:   []string{"developer"},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !response.Refused || len(response.Citations) != 0 {
		t.Fatalf("restricted evidence leaked into answer: %#v", response)
	}
}

func TestAskStoresRedactedTraceAndMetrics(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	store := NewMemoryTraceStore()
	service := NewService(knowledgeService, NewLocalExtractive(), store)
	query := "sensitive production question"
	response, err := service.Ask(context.Background(), Input{
		Query:   query,
		Project: "order",
		UserID:  "developer-1",
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	trace, err := service.Trace(context.Background(), response.TraceID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if trace.QueryHash == "" || trace.QueryLength != len(query) || trace.UserID != "developer-1" || !trace.Refused {
		t.Fatalf("unexpected trace: %#v", trace)
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("encode trace: %v", err)
	}
	if strings.Contains(string(encoded), query) {
		t.Fatalf("trace leaked raw query: %s", encoded)
	}
	metrics, err := service.Metrics(context.Background(), "order")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if metrics.Total != 1 || metrics.Succeeded != 1 || metrics.Refused != 1 || metrics.ByProvider["local-extractive"] != 1 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestAskReturnsTraceIDWhenProviderFails(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title:     "Startup",
		Content:   "Run go run ./cmd/server.",
		SourceURI: "git://app/README.md",
		Project:   "app",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	providerError := errors.New("provider unavailable")
	service := NewService(knowledgeService, fixedProvider{err: providerError}, NewMemoryTraceStore())
	_, err = service.Ask(context.Background(), Input{Query: "go run ./cmd/server", Project: "app", UserID: "developer-1"})
	if !errors.Is(err, providerError) {
		t.Fatalf("expected provider error, got %v", err)
	}
	traceID := ErrorTraceID(err)
	if traceID == "" {
		t.Fatal("provider error is missing trace ID")
	}
	trace, err := service.Trace(context.Background(), traceID)
	if err != nil {
		t.Fatalf("get failed trace: %v", err)
	}
	if trace.Status != "failed" || trace.ErrorCode != "generation_failed" {
		t.Fatalf("unexpected failed trace: %#v", trace)
	}
}

func TestAskStoresPricingSnapshotAndCost(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title: "Startup", Content: "Run go run ./cmd/server.", SourceURI: "git://app/README.md", Project: "app",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	store := NewMemoryTraceStore()
	service := NewService(knowledgeService, fixedProvider{draft: Draft{
		Answer: "Run the service.", CitationIDs: []string{"E1"},
		Usage: Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500},
	}}, store, Pricing{InputUSDPerMillionTokens: 2, OutputUSDPerMillionTokens: 4})
	response, err := service.Ask(context.Background(), Input{Query: "go run ./cmd/server", Project: "app"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	trace, err := service.Trace(context.Background(), response.TraceID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if math.Abs(trace.TotalCostUSD-0.004) > 0.0000001 || trace.InputRateUSD != 2 || trace.OutputRateUSD != 4 {
		t.Fatalf("unexpected cost trace: %#v", trace)
	}
	metrics, err := service.Metrics(context.Background(), "app")
	if err != nil || math.Abs(metrics.TotalCostUSD-0.004) > 0.0000001 {
		t.Fatalf("unexpected cost metrics: %#v, %v", metrics, err)
	}
}

func TestPruneTracesDeletesOnlyExpiredRecords(t *testing.T) {
	store := NewMemoryTraceStore()
	service := NewService(knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash()), NewLocalExtractive(), store)
	old := Trace{ID: "old", Provider: "test", Status: "succeeded", CreatedAt: time.Now().UTC().AddDate(0, 0, -31)}
	recent := Trace{ID: "recent", Provider: "test", Status: "succeeded", CreatedAt: time.Now().UTC()}
	_ = store.Save(context.Background(), old)
	_ = store.Save(context.Background(), recent)
	deleted, _, err := service.PruneTraces(context.Background(), 30)
	if err != nil || deleted != 1 {
		t.Fatalf("unexpected prune result: deleted=%d err=%v", deleted, err)
	}
	if _, err := store.Get(context.Background(), "old"); !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("expired trace was not deleted: %v", err)
	}
	if _, err := store.Get(context.Background(), "recent"); err != nil {
		t.Fatalf("recent trace was deleted: %v", err)
	}
}
