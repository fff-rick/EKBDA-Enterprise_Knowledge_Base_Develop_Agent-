package development

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type passingSecretScanner struct{}

func (passingSecretScanner) Name() string { return "enterprise-test" }
func (passingSecretScanner) Scan(context.Context, string) (SecretScanEvidence, error) {
	return SecretScanEvidence{Scanner: "enterprise-test", Passed: true, OutputSHA256: strings.Repeat("a", 64)}, nil
}

type recordingPullRequestPublisher struct {
	request PullRequestRequest
}

func (p *recordingPullRequestPublisher) Publish(_ context.Context, _ string, request PullRequestRequest) (string, error) {
	p.request = request
	return "https://git.example/app/pull/1", nil
}

func TestGitDelivererCreatesBranchCommitPushAndPullRequest(t *testing.T) {
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "workspaces")
	deliveryRoot := filepath.Join(base, "deliveries")
	repository := filepath.Join(workspaceRoot, "app")
	remote := filepath.Join(base, "remote.git")
	for _, directory := range []string{workspaceRoot, deliveryRoot, repository} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	runGitTest(t, base, "init", "--bare", "--initial-branch=main", remote)
	runGitTest(t, repository, "init", "--initial-branch=main")
	runGitTest(t, repository, "config", "user.email", "test@example.com")
	runGitTest(t, repository, "config", "user.name", "Delivery Test")
	writeRunnerFile(t, repository, "main.go", "package app\n\nvar version = 1\n")
	runGitTest(t, repository, "add", ".")
	runGitTest(t, repository, "commit", "--no-gpg-sign", "-m", "initial")
	runGitTest(t, repository, "remote", "add", "origin", remote)
	runGitTest(t, repository, "push", "-u", "origin", "main")
	baseline := strings.TrimSpace(runGitTest(t, repository, "rev-parse", "HEAD"))
	publisher := &recordingPullRequestPublisher{}
	deliverer, err := NewGitDeliverer(DeliveryConfig{
		Enabled: true, WorkspaceRoot: workspaceRoot, DeliveryRoot: deliveryRoot, Remote: "origin",
		AuthorName: "EKBDA Delivery Bot", AuthorEmail: "ekbda@example.invalid",
		Timeout: 30 * time.Second, AllowLocalRemote: true,
	}, passingSecretScanner{}, publisher)
	if err != nil {
		t.Fatalf("create deliverer: %v", err)
	}
	patch := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n package app\n \n-var version = 1\n+var version = 2\n"
	result := deliverer.Deliver(context.Background(), DeliveryRequest{
		DeliveryID: "delivery-1", SessionID: "session-1", Repository: "app", Project: "app",
		BaselineCommit: baseline, BaselineBranch: "main", Branch: "codex/app/session-1",
		PatchHash: strings.Repeat("b", 64), Patch: patch,
		Files:   []FileChange{{Path: "main.go", Operation: "modify", Additions: 1, Deletions: 1}},
		Summary: "update version", Actor: "approver-1",
	})
	if result.Status != DeliveryPassed || !result.BranchPushed || result.Commit == "" || result.PullRequestURL == "" || result.SecretScan == nil || !result.SecretScan.Passed || !result.WorkingCopyRemoved {
		t.Fatalf("unexpected delivery result: %#v", result)
	}
	remoteCommit := strings.TrimSpace(runGitTest(t, base, "--git-dir="+remote, "rev-parse", "refs/heads/codex/app/session-1"))
	if remoteCommit != result.Commit || publisher.request.Head != "codex/app/session-1" || publisher.request.Base != "main" {
		t.Fatalf("unexpected remote or PR evidence: commit=%q request=%#v", remoteCommit, publisher.request)
	}
	content, err := os.ReadFile(filepath.Join(repository, "main.go"))
	if err != nil || strings.Contains(string(content), "version = 2") {
		t.Fatalf("source repository was modified: %q, %v", content, err)
	}
	if status := strings.TrimSpace(runGitTest(t, repository, "status", "--porcelain")); status != "" {
		t.Fatalf("source repository became dirty: %q", status)
	}
	entries, err := os.ReadDir(deliveryRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("delivery working copy was not removed: %#v, %v", entries, err)
	}
}

func TestDeliveryRemoteCredentialSafety(t *testing.T) {
	if validDeliveryRemote("https://user:token@git.example/org/repo.git", false) {
		t.Fatal("remote URL containing credentials must be rejected")
	}
	value, err := remoteWithUsername("https://git.example/org/repo.git", "delivery-bot@example.com")
	if err != nil || value != "https://delivery-bot%40example.com@git.example/org/repo.git" {
		t.Fatalf("unexpected username-only remote: %q, %v", value, err)
	}
	if _, err := remoteWithUsername("ssh://git@git.example/org/repo.git", "delivery-bot"); err == nil {
		t.Fatal("token AskPass must only be used with HTTPS remotes")
	}
}
