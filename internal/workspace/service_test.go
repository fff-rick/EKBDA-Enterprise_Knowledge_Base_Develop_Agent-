package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ekbda/internal/standards"
)

func runTestGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeTestFile(t *testing.T, root, name string, content []byte) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(fullPath, content, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func newRepositoryFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "order-service")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	runTestGit(t, repository, "init", "--initial-branch=main")
	runTestGit(t, repository, "config", "user.email", "test@example.com")
	runTestGit(t, repository, "config", "user.name", "Workspace Test")
	writeTestFile(t, repository, ".gitignore", []byte(".env\n"))
	writeTestFile(t, repository, "README.md", []byte("# Order Service\n"))
	writeTestFile(t, repository, "go.mod", []byte("module company/order\n"))
	writeTestFile(t, repository, "internal/order.go", []byte("package internal\n"))
	writeTestFile(t, repository, "internal/order_test.go", []byte("package internal\n"))
	writeTestFile(t, repository, "assets/logo.bin", []byte{0, 1, 2, 3})
	runTestGit(t, repository, "add", ".")
	runTestGit(t, repository, "commit", "--no-gpg-sign", "-m", "initial")
	return root, repository
}

func newWorkspaceService(t *testing.T, root string) (*Service, *standards.Service) {
	t.Helper()
	standardService := standards.NewService(standards.NewMemoryStore())
	_, err := standardService.CreatePackage(context.Background(), standards.CreatePackageInput{
		Name: "go-workspace", Scope: standards.ScopeTechnology, Selector: "go", Owner: "platform",
		Rules: []standards.Rule{
			{ID: "readme", Title: "README", Category: standards.CategoryDirectory, Level: standards.LevelBlock, Check: &standards.RuleCheck{Type: standards.CheckRequiredPath, Pattern: "README.md"}},
			{ID: "tests", Title: "Tests", Category: standards.CategoryTesting, Level: standards.LevelBlock, Check: &standards.RuleCheck{Type: standards.CheckMinimumMatch, Target: `_test\.go$`, Minimum: 1}},
		},
	}, "admin")
	if err != nil {
		t.Fatalf("create standards: %v", err)
	}
	service, err := New(root, standardService, NewMemoryStore())
	if err != nil {
		t.Fatalf("create workspace service: %v", err)
	}
	return service, standardService
}

func TestValidateRepositoryCapturesGitStateAndStandards(t *testing.T) {
	root, repository := newRepositoryFixture(t)
	service, _ := newWorkspaceService(t, root)
	writeTestFile(t, repository, "internal/order.go", []byte("package internal\n\nconst changed = true\n"))
	writeTestFile(t, repository, "internal/new_file.go", []byte("package internal\n"))
	writeTestFile(t, repository, ".env", []byte("SECRET=not-read\n"))

	result, err := service.Validate(context.Background(), ValidateInput{
		Repository: "order-service", Project: "order-service", Technology: "go",
	}, "developer-1")
	if err != nil {
		t.Fatalf("validate repository: %v", err)
	}
	if !result.Repository.Passed || !result.Standards.Passed || !result.Repository.Dirty {
		t.Fatalf("unexpected validation result: %#v", result)
	}
	if result.Repository.TrackedCount != 6 || result.Repository.UntrackedCount != 1 || result.Repository.BinaryCount != 1 || result.Repository.ChangedCount != 2 {
		t.Fatalf("unexpected repository counts: %#v", result.Repository)
	}
	if result.Repository.HeadCommit == "" || result.Repository.Branch != "main" || result.Repository.InputHash == "" || result.Repository.StandardsReportID != result.Standards.ID {
		t.Fatalf("missing repository audit metadata: %#v", result.Repository)
	}
	for _, change := range result.Repository.Changes {
		if change.Path == ".env" {
			t.Fatalf("ignored secret file entered snapshot: %#v", result.Repository.Changes)
		}
	}
	stored, err := service.Get(context.Background(), result.Repository.ID)
	if err != nil || stored.Repository.InputHash != result.Repository.InputHash || stored.Standards.ID != result.Standards.ID {
		t.Fatalf("get stored validation: %#v err=%v", stored, err)
	}
}

func TestValidateRepositoryTreatsDeletedTrackedFileAsMissing(t *testing.T) {
	root, repository := newRepositoryFixture(t)
	service, _ := newWorkspaceService(t, root)
	if err := os.Remove(filepath.Join(repository, "README.md")); err != nil {
		t.Fatalf("delete README: %v", err)
	}
	result, err := service.Validate(context.Background(), ValidateInput{
		Repository: "order-service", Project: "order-service", Technology: "go",
	}, "developer-1")
	if err != nil {
		t.Fatalf("validate repository: %v", err)
	}
	if result.Repository.Passed || result.Standards.BlockingCount != 1 {
		t.Fatalf("deleted required file was not detected: %#v", result)
	}
	foundDeletion := false
	for _, change := range result.Repository.Changes {
		if change.Path == "README.md" && change.WorktreeStatus == "D" {
			foundDeletion = true
		}
	}
	if !foundDeletion {
		t.Fatalf("deleted path missing from changes: %#v", result.Repository.Changes)
	}
}

func TestValidateRepositoryRejectsTraversalAndSubdirectory(t *testing.T) {
	root, _ := newRepositoryFixture(t)
	service, _ := newWorkspaceService(t, root)
	for _, repository := range []string{"../outside", "order-service/internal"} {
		_, err := service.Validate(context.Background(), ValidateInput{
			Repository: repository, Project: "order-service", Technology: "go",
		}, "developer")
		if !errors.Is(err, ErrInvalidRepository) {
			t.Fatalf("expected invalid repository for %q, got %v", repository, err)
		}
	}
}

func TestDisabledWorkspaceFailsClosed(t *testing.T) {
	standardService := standards.NewService(standards.NewMemoryStore())
	service, err := New("", standardService, NewMemoryStore())
	if err != nil {
		t.Fatalf("create disabled workspace: %v", err)
	}
	_, err = service.Validate(context.Background(), ValidateInput{Repository: ".", Project: "app", Technology: "go"}, "developer")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected disabled error, got %v", err)
	}
}

func TestParseNULPathsPreservesSpaces(t *testing.T) {
	paths, err := parseNULPaths([]byte("README.md\x00docs/file name.md\x00"))
	if err != nil || len(paths) != 2 || paths[1] != "docs/file name.md" {
		t.Fatalf("unexpected parsed paths: %#v err=%v", paths, err)
	}
}
