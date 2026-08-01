package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ekbda/internal/embedding"
	"ekbda/internal/knowledge"
)

func newKnowledgeService() *knowledge.Service {
	return knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
}

func TestImportCreatesSkipsAndUpdatesDocuments(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "docs", "README.md"), "# 订单服务\n\n运行服务前需要启动 PostgreSQL。")
	writeTestFile(t, filepath.Join(root, "node_modules", "ignored.md"), "不应导入")

	knowledgeService := newKnowledgeService()
	service := New(root, knowledgeService, NewMemoryJobStore())
	input := Input{Path: ".", Project: "order-service", BusinessDomain: "交易"}

	first, err := service.Import(context.Background(), input)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.Status != "completed" || first.Scanned != 1 || first.Created != 1 || first.Updated != 0 || first.Skipped != 0 {
		t.Fatalf("unexpected first report: %#v", first)
	}

	second, err := service.Import(context.Background(), input)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.Scanned != 1 || second.Skipped != 1 || second.Created != 0 {
		t.Fatalf("unexpected second report: %#v", second)
	}

	writeTestFile(t, filepath.Join(root, "docs", "README.md"), "# 订单服务\n\n运行服务前需要启动 PostgreSQL。\n\n新增健康检查说明。")
	third, err := service.Import(context.Background(), input)
	if err != nil {
		t.Fatalf("third import: %v", err)
	}
	if third.Updated != 1 || len(third.Files) != 1 || third.Files[0].Version != 2 {
		t.Fatalf("unexpected third report: %#v", third)
	}

	results, err := knowledgeService.Search(context.Background(), knowledge.SearchInput{
		Query:   "健康检查",
		Project: "order-service",
	})
	if err != nil {
		t.Fatalf("search imported content: %v", err)
	}
	if len(results) != 1 || results[0].Citation.Version != 2 || results[0].Citation.SourceURI != "file:///docs/README.md" {
		t.Fatalf("unexpected search results: %#v", results)
	}

	if err := os.Remove(filepath.Join(root, "docs", "README.md")); err != nil {
		t.Fatalf("remove imported file: %v", err)
	}
	fourth, err := service.Import(context.Background(), input)
	if err != nil {
		t.Fatalf("fourth import: %v", err)
	}
	if fourth.Deleted != 1 || len(fourth.Files) != 1 || fourth.Files[0].Action != knowledge.ImportActionDeleted {
		t.Fatalf("unexpected deletion report: %#v", fourth)
	}
	results, err = knowledgeService.Search(context.Background(), knowledge.SearchInput{Query: "健康检查", Project: "order-service"})
	if err != nil {
		t.Fatalf("search after deletion: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected deleted document to be hidden, got %#v", results)
	}

	writeTestFile(t, filepath.Join(root, "docs", "README.md"), "# 订单服务\n\n运行服务前需要启动 PostgreSQL。\n\n新增健康检查说明。")
	fifth, err := service.Import(context.Background(), input)
	if err != nil {
		t.Fatalf("fifth import: %v", err)
	}
	if fifth.Updated != 1 || fifth.Files[0].Version != 4 {
		t.Fatalf("unexpected restored report: %#v", fifth)
	}
	versions, err := knowledgeService.Versions(context.Background(), fifth.Files[0].DocumentID)
	if err != nil {
		t.Fatalf("list document versions: %v", err)
	}
	if len(versions) != 4 || versions[0].Status != knowledge.DocumentStatusActive || versions[1].Status != knowledge.DocumentStatusDeleted {
		t.Fatalf("unexpected version history: %#v", versions)
	}
}

func TestImportRejectsPathOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "allowed")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create import root: %v", err)
	}
	service := New(root, newKnowledgeService(), NewMemoryJobStore())
	_, err := service.Import(context.Background(), Input{Path: "..", Project: "project"})
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestImportReportsInvalidUTF8File(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "invalid.txt"), []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatalf("write invalid file: %v", err)
	}
	service := New(root, newKnowledgeService(), NewMemoryJobStore())
	report, err := service.Import(context.Background(), Input{Path: ".", Project: "project"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Status != "completed_with_errors" || report.Failed != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
