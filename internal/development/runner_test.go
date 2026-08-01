package development

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ekbda/internal/standards"
)

func TestLocalRunnerExecutesApprovedPatchInIsolatedClone(t *testing.T) {
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "workspaces")
	executionRoot := filepath.Join(base, "executions")
	repository := filepath.Join(workspaceRoot, "app")
	for _, directory := range []string{workspaceRoot, executionRoot, repository} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	runGitTest(t, repository, "init", "--initial-branch=main")
	runGitTest(t, repository, "config", "user.email", "test@example.com")
	runGitTest(t, repository, "config", "user.name", "Runner Test")
	writeRunnerFile(t, repository, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeRunnerFile(t, repository, "main.go", "package app\n\nfunc Add(a, b int) int { return a - b }\n")
	runGitTest(t, repository, "add", ".")
	runGitTest(t, repository, "commit", "--no-gpg-sign", "-m", "initial")
	baseline := strings.TrimSpace(runGitTest(t, repository, "rev-parse", "HEAD"))

	standardStore := standards.NewMemoryStore()
	standardService := standards.NewService(standardStore)
	if _, err := standardService.CreatePackage(context.Background(), standards.CreatePackageInput{
		Name: "go-base", Scope: standards.ScopeTechnology, Selector: "go", Owner: "platform",
		Rules: []standards.Rule{{ID: "main-required", Title: "main.go required", Category: standards.CategoryDirectory, Level: standards.LevelBlock, Check: &standards.RuleCheck{Type: standards.CheckRequiredPath, Pattern: "main.go"}}},
	}, "admin"); err != nil {
		t.Fatalf("create standards: %v", err)
	}
	runner, err := NewLocalRunner(true, workspaceRoot, executionRoot, standardService, 30*time.Second)
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n package app\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n"
	result := runner.Run(context.Background(), RunRequest{
		ExecutionID: "execution-1", SessionID: "session-1", Repository: "app", Project: "app", Technology: "go",
		BaselineCommit: baseline, PatchHash: "patch-hash", Patch: patch,
		Files:    []FileChange{{Path: "main.go", Operation: "modify", Additions: 1, Deletions: 1}},
		Commands: []Command{{ID: "go-test-all"}}, Actor: "developer-1",
	})
	if result.Status != ExecutionPassed || !result.SecretScanPassed || !result.StandardsPassed || result.StandardsReportID == "" || len(result.Commands) != 1 || result.Commands[0].ExitCode != 0 || !result.IsolatedCopyRemoved {
		t.Fatalf("unexpected execution result: %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(repository, "main.go"))
	if err != nil || strings.Contains(string(content), "a + b") {
		t.Fatalf("source repository was modified: %q, %v", content, err)
	}
	if status := strings.TrimSpace(runGitTest(t, repository, "status", "--porcelain")); status != "" {
		t.Fatalf("source repository became dirty: %q", status)
	}
	entries, err := os.ReadDir(executionRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("isolated copy was not removed: %#v, %v", entries, err)
	}
}

func TestLocalRunnerDefenseInDepthRejectsSecretAndOverlappingRoots(t *testing.T) {
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "workspaces")
	executionRoot := filepath.Join(base, "executions")
	repository := filepath.Join(workspaceRoot, "app")
	for _, directory := range []string{workspaceRoot, executionRoot, repository} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create directory: %v", err)
		}
	}
	runGitTest(t, repository, "init", "--initial-branch=main")
	runGitTest(t, repository, "config", "user.email", "test@example.com")
	runGitTest(t, repository, "config", "user.name", "Runner Test")
	writeRunnerFile(t, repository, "config.go", "package app\n\nvar value = \"safe-value\"\n")
	runGitTest(t, repository, "add", ".")
	runGitTest(t, repository, "commit", "--no-gpg-sign", "-m", "initial")
	baseline := strings.TrimSpace(runGitTest(t, repository, "rev-parse", "HEAD"))
	standardService := standards.NewService(standards.NewMemoryStore())
	runner, err := NewLocalRunner(true, workspaceRoot, executionRoot, standardService, 10*time.Second)
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	patch := "diff --git a/config.go b/config.go\n--- a/config.go\n+++ b/config.go\n@@ -1,3 +1,3 @@\n package app\n \n-var value = \"safe-value\"\n+var api_key = \"abcdefghijk\"\n"
	result := runner.Run(context.Background(), RunRequest{
		ExecutionID: "execution-2", Repository: "app", Project: "app", Technology: "go", BaselineCommit: baseline,
		Patch: patch, Files: []FileChange{{Path: "config.go", Operation: "modify"}}, Actor: "developer-1",
	})
	if result.Status != ExecutionFailed || result.ErrorCode != "secret_scan_failed" || result.SecretScanPassed || !result.IsolatedCopyRemoved {
		t.Fatalf("secret gate result: %#v", result)
	}
	overlappingRoot := filepath.Join(workspaceRoot, "executions")
	if err := os.Mkdir(overlappingRoot, 0o700); err != nil {
		t.Fatalf("create overlapping root: %v", err)
	}
	if _, err := NewLocalRunner(true, workspaceRoot, overlappingRoot, standardService, time.Second); err == nil {
		t.Fatal("overlapping execution root must be rejected")
	}
	stale := filepath.Join(executionRoot, executionDirectoryPrefix+"stale-123")
	active := filepath.Join(executionRoot, executionDirectoryPrefix+"active-123")
	for _, directory := range []string{stale, active} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create cleanup fixture: %v", err)
		}
	}
	if err := runner.CleanupStale(context.Background(), []string{"stale"}); err != nil {
		t.Fatalf("cleanup stale: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale execution directory remains: %v", err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active execution directory was removed: %v", err)
	}
}

func runGitTest(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return string(output)
}

func writeRunnerFile(t *testing.T, repository, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repository, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
