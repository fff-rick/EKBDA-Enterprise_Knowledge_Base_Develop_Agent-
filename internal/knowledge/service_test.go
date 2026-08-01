package knowledge

import (
	"context"
	"testing"

	"ekbda/internal/embedding"
	"ekbda/internal/reranking"
)

func newTestService() *Service {
	return NewService(NewMemoryRepository(), embedding.NewLocalHash())
}

func TestSearchFiltersRestrictedDocumentsByRole(t *testing.T) {
	service := newTestService()
	ctx := context.Background()
	_, err := service.Create(ctx, CreateDocumentInput{
		Title:          "订单退款规则",
		Content:        "退款申请需要经过财务复核。",
		SourceURI:      "wiki://orders/refund",
		Project:        "order-service",
		Classification: ClassificationRestricted,
		AllowedRoles:   []string{"finance"},
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	withoutRole, err := service.Search(ctx, SearchInput{Query: "退款", Roles: []string{"developer"}})
	if err != nil {
		t.Fatalf("search without role: %v", err)
	}
	if len(withoutRole) != 0 {
		t.Fatalf("expected no restricted results, got %d", len(withoutRole))
	}

	withRole, err := service.Search(ctx, SearchInput{Query: "退款", Roles: []string{"finance"}})
	if err != nil {
		t.Fatalf("search with role: %v", err)
	}
	if len(withRole) != 1 {
		t.Fatalf("expected one result, got %d", len(withRole))
	}
	if withRole[0].Citation.SourceURI != "wiki://orders/refund" {
		t.Fatalf("unexpected citation: %#v", withRole[0].Citation)
	}
}

func TestCreateRejectsRestrictedDocumentWithoutRoles(t *testing.T) {
	service := newTestService()
	_, err := service.Create(context.Background(), CreateDocumentInput{
		Title:          "受限文档",
		Content:        "内容",
		SourceURI:      "wiki://restricted",
		Project:        "project-a",
		Classification: ClassificationRestricted,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSearchFiltersByProject(t *testing.T) {
	service := newTestService()
	ctx := context.Background()
	for _, project := range []string{"project-a", "project-b"} {
		_, err := service.Create(ctx, CreateDocumentInput{
			Title:     "启动说明",
			Content:   "使用 go run 启动服务",
			SourceURI: "git://" + project + "/README.md",
			Project:   project,
		})
		if err != nil {
			t.Fatalf("create document: %v", err)
		}
	}
	results, err := service.Search(ctx, SearchInput{Query: "启动", Project: "project-a"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Citation.SourceURI != "git://project-a/README.md" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSearchIncludesVectorOnlyMatch(t *testing.T) {
	service := newTestService()
	ctx := context.Background()
	_, err := service.Create(ctx, CreateDocumentInput{
		Title:     "售后规则",
		Content:   "顾客可以提交退款申请，审核通过后原路退回。",
		SourceURI: "wiki://support/refund",
		Project:   "support",
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	results, err := service.Search(ctx, SearchInput{Query: "退款办理", Project: "support"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected a vector match")
	}
	if results[0].KeywordScore != 0 || results[0].VectorScore < minimumVectorScore {
		t.Fatalf("expected vector-only signals, got %#v", results[0])
	}
	if results[0].Reranker != "local-weighted-v1" || results[0].FusionScore <= 0 || results[0].RerankScore <= 0 {
		t.Fatalf("expected rerank metadata, got %#v", results[0])
	}
}

func TestSearchUsesRepositoryCandidatePushdown(t *testing.T) {
	repository := &pushedDownRepository{MemoryRepository: NewMemoryRepository()}
	service := NewService(repository, embedding.NewLocalHash())
	results, err := service.Search(context.Background(), SearchInput{
		Query: "startup", Project: "app", Roles: []string{"developer"}, Limit: 2,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !repository.called || repository.input.Project != "app" || len(repository.input.Roles) != 1 {
		t.Fatalf("candidate pushdown did not receive search scope: %#v", repository.input)
	}
	if len(results) != 1 || results[0].Citation.SourceURI != "git://app/README.md" {
		t.Fatalf("unexpected pushed-down results: %#v", results)
	}
}

func TestSearchAppliesConfiguredReranker(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository, embedding.NewLocalHash(), reverseReranker{})
	ctx := context.Background()
	for _, source := range []string{"first", "second"} {
		_, err := service.Create(ctx, CreateDocumentInput{
			Title: "Startup " + source, Content: "run the startup command",
			SourceURI: "git://" + source, Project: "app",
		})
		if err != nil {
			t.Fatalf("create document: %v", err)
		}
	}
	results, err := service.Search(ctx, SearchInput{Query: "startup", Project: "app", Limit: 2})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 || results[0].Citation.SourceURI != "git://second" || results[0].Reranker != "reverse" {
		t.Fatalf("configured reranker was not applied: %#v", results)
	}
}

type pushedDownRepository struct {
	*MemoryRepository
	called bool
	input  candidateSearchInput
}

func (r *pushedDownRepository) SearchCandidates(_ context.Context, input candidateSearchInput) ([]searchCandidate, error) {
	r.called = true
	r.input = input
	return []searchCandidate{{
		title: "Startup", content: "run the startup command",
		result: SearchResult{
			KeywordScore: 4, VectorScore: 0.8, Snippet: "run the startup command",
			Citation: Citation{DocumentID: "document", Title: "Startup", SourceURI: "git://app/README.md", Version: 1},
		},
	}}, nil
}

type reverseReranker struct{}

func (reverseReranker) Name() string { return "reverse" }
func (reverseReranker) Rerank(_ context.Context, _ string, candidates []reranking.Candidate) (reranking.Output, error) {
	scores := make([]float64, len(candidates))
	for index := range scores {
		scores[index] = float64(index)
	}
	return reranking.Output{Scores: scores, Provider: "reverse"}, nil
}
