package repositorysync

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ekbda/internal/embedding"
	"ekbda/internal/knowledge"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

func TestSyncRepositoryRedactsAndTracksCommitDiff(t *testing.T) {
	root, repository := newGitRepository(t)
	writeFile(t, repository, "README.md", "# Order\npassword = super-secret\n")
	writeFile(t, repository, "internal/order.go", "package internal\n")
	writeFile(t, repository, "config/secrets.json", `{"token":"must-not-enter"}`)
	git(t, repository, "add", ".")
	git(t, repository, "commit", "--no-gpg-sign", "-m", "initial")

	service, knowledgeRepository := newSyncService(t, root)
	syncInput := Input{
		Repository: "order-service", Project: "order-service", BusinessDomain: "trade",
		Classification: knowledge.ClassificationInternal,
	}
	first, err := service.Sync(context.Background(), syncInput, "developer-1")
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if first.Status != StatusCompleted || first.Created != 2 || first.RedactionCount != 1 || first.SensitiveFilesSkipped != 1 || first.HeadCommit == "" || len(first.CommitChanges) != 3 {
		t.Fatalf("unexpected first report: %#v", first)
	}
	documents, err := knowledgeRepository.List(context.Background())
	if err != nil || len(documents) != 2 {
		t.Fatalf("list synced knowledge: %#v err=%v", documents, err)
	}
	for _, document := range documents {
		if strings.Contains(document.Content, "super-secret") || strings.Contains(document.Content, "must-not-enter") {
			t.Fatalf("raw secret entered knowledge: %#v", document)
		}
	}
	if !containsDocumentContent(documents, "[REDACTED:secret]") {
		t.Fatalf("redaction marker missing from knowledge: %#v", documents)
	}

	second, err := service.Sync(context.Background(), syncInput, "developer-1")
	if err != nil || second.Skipped != 3 || len(second.CommitChanges) != 0 || second.PreviousHeadCommit != first.HeadCommit {
		t.Fatalf("unchanged sync: %#v err=%v", second, err)
	}

	writeFile(t, repository, "README.md", "# Order v2\n")
	if err := os.Remove(filepath.Join(repository, "internal", "order.go")); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	writeFile(t, repository, "docs/runbook.md", "Run the order service\n")
	git(t, repository, "add", "-A")
	git(t, repository, "commit", "--no-gpg-sign", "-m", "update docs")

	third, err := service.Sync(context.Background(), syncInput, "developer-1")
	if err != nil {
		t.Fatalf("incremental sync: %v", err)
	}
	if third.Updated != 1 || third.Created != 1 || third.Deleted != 1 || third.PreviousHeadCommit != first.HeadCommit {
		t.Fatalf("unexpected incremental report: %#v", third)
	}
	wantChanges := map[string]string{"README.md": "M", "docs/runbook.md": "A", "internal/order.go": "D"}
	for _, change := range third.CommitChanges {
		if wantChanges[change.Path] != change.Status {
			t.Fatalf("unexpected commit change: %#v", change)
		}
		delete(wantChanges, change.Path)
	}
	if len(wantChanges) != 0 {
		t.Fatalf("missing commit changes: %#v", wantChanges)
	}

	writeFile(t, repository, "README.md", "uncommitted\n")
	if _, err := service.Sync(context.Background(), syncInput, "developer-1"); !errors.Is(err, ErrDirtyRepository) {
		t.Fatalf("dirty repository must be rejected: %v", err)
	}
}

func TestSyncGuardRejectsConcurrentRepositoryRun(t *testing.T) {
	service := New(nil, nil, NewMemoryStore())
	if !service.begin("project\x00repo") {
		t.Fatal("first sync guard acquisition failed")
	}
	if service.begin("project\x00repo") {
		t.Fatal("second sync guard acquisition must fail")
	}
	service.end("project\x00repo")
	if !service.begin("project\x00repo") {
		t.Fatal("sync guard was not released")
	}
	service.end("project\x00repo")
}

func TestDocumentHashIncludesKnowledgeAccessMetadata(t *testing.T) {
	internalHash, err := documentHash("content", Input{Classification: knowledge.ClassificationInternal})
	if err != nil {
		t.Fatalf("hash internal document: %v", err)
	}
	restrictedHash, err := documentHash("content", Input{
		Classification: knowledge.ClassificationRestricted, AllowedRoles: []string{"team_order"},
	})
	if err != nil {
		t.Fatalf("hash restricted document: %v", err)
	}
	if internalHash == restrictedHash {
		t.Fatal("classification and allowed roles must participate in incremental hash")
	}
}

func newSyncService(t *testing.T, root string) (*Service, *knowledge.MemoryRepository) {
	t.Helper()
	standardService := standards.NewService(standards.NewMemoryStore())
	workspaceService, err := workspace.New(root, standardService, workspace.NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace service: %v", err)
	}
	repository := knowledge.NewMemoryRepository()
	knowledgeService := knowledge.NewService(repository, embedding.NewLocalHash())
	return New(workspaceService, knowledgeService, NewMemoryStore()), repository
}

func newGitRepository(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "order-service")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	git(t, repository, "init", "--initial-branch=main")
	git(t, repository, "config", "user.email", "sync@example.com")
	git(t, repository, "config", "user.name", "Sync Test")
	return root, repository
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func git(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func containsDocumentContent(documents []knowledge.Document, text string) bool {
	for _, document := range documents {
		if strings.Contains(document.Content, text) {
			return true
		}
	}
	return false
}
