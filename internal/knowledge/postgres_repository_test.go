package knowledge

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"ekbda/internal/embedding"
)

func TestVectorLiteralRejectsNonFiniteValues(t *testing.T) {
	if _, err := vectorLiteral([]float32{1, float32(math.Inf(1))}); err == nil {
		t.Fatal("expected non-finite vector validation error")
	}
	literal, err := vectorLiteral([]float32{1, -0.5})
	if err != nil || literal != "[1,-0.5]" {
		t.Fatalf("unexpected vector literal %q: %v", literal, err)
	}
}

func TestValidateEmbeddingDimensions(t *testing.T) {
	err := validateEmbeddingDimensions(Document{Chunks: []Chunk{{Index: 2, Embedding: []float32{1, 2}}}}, 3)
	if err == nil {
		t.Fatal("expected embedding dimension mismatch")
	}
}

func TestPostgresRepositoryRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repository, err := NewPostgresRepository(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL repository: %v", err)
	}
	defer repository.Close()

	service := NewService(repository, embedding.NewLocalHash())
	document, err := service.Create(ctx, CreateDocumentInput{
		Title:          "PostgreSQL 集成测试文档",
		Content:        "该文档验证知识持久化和中文检索。",
		SourceURI:      "test://postgres-round-trip",
		Project:        "integration-test",
		Classification: ClassificationInternal,
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repository.db.ExecContext(context.Background(), "DELETE FROM knowledge_documents WHERE id = $1", document.ID)
	})
	var dimensions int
	if err := repository.db.QueryRowContext(ctx, `
		SELECT vector_dims(embedding_vector)
		FROM knowledge_chunks
		WHERE document_id = $1
		LIMIT 1`, document.ID).Scan(&dimensions); err != nil {
		t.Fatalf("read pgvector dimensions: %v", err)
	}
	if dimensions != 384 {
		t.Fatalf("unexpected pgvector dimensions: %d", dimensions)
	}

	results, err := service.Search(ctx, SearchInput{Query: "中文检索", Project: "integration-test"})
	if err != nil {
		t.Fatalf("search document: %v", err)
	}
	if len(results) != 1 || results[0].Citation.DocumentID != document.ID {
		t.Fatalf("unexpected search results: %#v", results)
	}

	restricted, err := service.Create(ctx, CreateDocumentInput{
		Title: "受限中文检索", Content: "该文档包含中文检索和财务机密。",
		SourceURI: "test://postgres-restricted-" + newID(), Project: "integration-test",
		Classification: ClassificationRestricted, AllowedRoles: []string{"finance"},
	})
	if err != nil {
		t.Fatalf("create restricted document: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repository.db.ExecContext(context.Background(), "DELETE FROM knowledge_documents WHERE id = $1", restricted.ID)
	})
	developerResults, err := service.Search(ctx, SearchInput{Query: "财务机密", Project: "integration-test", Roles: []string{"developer"}})
	if err != nil {
		t.Fatalf("search without restricted role: %v", err)
	}
	for _, result := range developerResults {
		if result.Citation.DocumentID == restricted.ID {
			t.Fatalf("pgvector candidate query leaked restricted document: %#v", result)
		}
	}
}
