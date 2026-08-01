package development

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const deliveryDirectoryPrefix = "ekbda-delivery-"

var ErrInvalidDeliveryConfig = errors.New("invalid controlled development delivery configuration")

type DeliveryRequest struct {
	DeliveryID     string
	SessionID      string
	Repository     string
	Project        string
	BaselineCommit string
	BaselineBranch string
	Branch         string
	PatchHash      string
	Patch          string
	Files          []FileChange
	Summary        string
	Actor          string
}

type Deliverer interface {
	Enabled() bool
	Deliver(context.Context, DeliveryRequest) Delivery
	RecoveryGracePeriod() time.Duration
	CleanupStale(context.Context, []string) error
}

type disabledDeliverer struct{}

func (disabledDeliverer) Enabled() bool                                     { return false }
func (disabledDeliverer) Deliver(context.Context, DeliveryRequest) Delivery { return Delivery{} }
func (disabledDeliverer) RecoveryGracePeriod() time.Duration                { return 0 }
func (disabledDeliverer) CleanupStale(context.Context, []string) error      { return nil }

type PullRequestRequest struct {
	SessionID string
	Project   string
	Head      string
	Base      string
	Title     string
	Body      string
}

type PullRequestPublisher interface {
	Publish(context.Context, string, PullRequestRequest) (string, error)
}

type DeliveryConfig struct {
	Enabled          bool
	WorkspaceRoot    string
	DeliveryRoot     string
	Remote           string
	AuthorName       string
	AuthorEmail      string
	Username         string
	Token            string
	Timeout          time.Duration
	AllowLocalRemote bool
}

type GitDeliverer struct {
	enabled          bool
	workspaceRoot    string
	deliveryRoot     string
	remote           string
	authorName       string
	authorEmail      string
	username         string
	token            string
	timeout          time.Duration
	gitBinary        string
	scanner          SecretScanner
	publisher        PullRequestPublisher
	allowLocalRemote bool
}

func NewGitDeliverer(config DeliveryConfig, scanner SecretScanner, publisher PullRequestPublisher) (*GitDeliverer, error) {
	deliverer := &GitDeliverer{enabled: config.Enabled}
	if !config.Enabled {
		return deliverer, nil
	}
	config.Remote = strings.TrimSpace(config.Remote)
	config.AuthorName = strings.TrimSpace(config.AuthorName)
	config.AuthorEmail = strings.TrimSpace(config.AuthorEmail)
	if config.Remote == "" {
		config.Remote = "origin"
	}
	if scanner == nil || publisher == nil || config.Timeout < time.Second || !developmentKeyPattern.MatchString(config.Remote) || config.AuthorName == "" || config.AuthorEmail == "" || strings.ContainsAny(config.AuthorName+config.AuthorEmail, "\r\n") || (config.Token == "") != (config.Username == "") {
		return nil, ErrInvalidDeliveryConfig
	}
	workspaceRoot, err := resolveExistingDirectory(config.WorkspaceRoot)
	if err != nil {
		return nil, ErrInvalidDeliveryConfig
	}
	deliveryRoot, err := resolveExistingDirectory(config.DeliveryRoot)
	if err != nil || pathsOverlap(workspaceRoot, deliveryRoot) {
		return nil, ErrInvalidDeliveryConfig
	}
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return nil, ErrInvalidDeliveryConfig
	}
	return &GitDeliverer{
		enabled: true, workspaceRoot: workspaceRoot, deliveryRoot: deliveryRoot, remote: config.Remote,
		authorName: config.AuthorName, authorEmail: config.AuthorEmail, username: config.Username,
		token: config.Token, timeout: config.Timeout, gitBinary: gitBinary, scanner: scanner,
		publisher: publisher, allowLocalRemote: config.AllowLocalRemote,
	}, nil
}

func (d *GitDeliverer) Enabled() bool { return d != nil && d.enabled }

func (d *GitDeliverer) RecoveryGracePeriod() time.Duration {
	if !d.Enabled() {
		return 0
	}
	return d.timeout + 30*time.Second
}

func (d *GitDeliverer) CleanupStale(_ context.Context, deliveryIDs []string) error {
	if !d.Enabled() || len(deliveryIDs) == 0 {
		return nil
	}
	prefixes := make([]string, 0, len(deliveryIDs))
	for _, id := range deliveryIDs {
		if !validExecutionID(id) {
			return ErrInvalidDeliveryConfig
		}
		prefixes = append(prefixes, deliveryDirectoryPrefix+id+"-")
	}
	entries, err := os.ReadDir(d.deliveryRoot)
	if err != nil {
		return fmt.Errorf("list controlled delivery root: %w", err)
	}
	for _, entry := range entries {
		matched := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(entry.Name(), prefix) {
				matched = true
				break
			}
		}
		if !matched || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		target := filepath.Join(d.deliveryRoot, entry.Name())
		if !withinPath(d.deliveryRoot, target) {
			return ErrInvalidDeliveryConfig
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove stale controlled delivery directory: %w", err)
		}
	}
	return nil
}

func (d *GitDeliverer) Deliver(ctx context.Context, request DeliveryRequest) (result Delivery) {
	started := time.Now().UTC()
	result = Delivery{
		ID: request.DeliveryID, Status: DeliveryFailed, Branch: request.Branch, Remote: d.remote,
		DeliveredBy: request.Actor, StartedAt: started,
	}
	defer func() {
		result.FinishedAt = time.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	}()
	fail := func(code, message string) Delivery {
		result.ErrorCode = code
		result.ErrorMessage = message
		return result
	}
	if !d.Enabled() {
		return fail("delivery_disabled", ErrDeliveryDisabled.Error())
	}
	overallContext, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	ctx = overallContext
	if !validExecutionID(request.DeliveryID) || request.BaselineBranch == "" || request.Branch == "" || request.Branch == request.BaselineBranch {
		return fail("delivery_policy_failed", "invalid controlled delivery identity or branch")
	}
	source, err := d.resolveSourceRepository(ctx, request.Repository)
	if err != nil {
		return fail("source_validation_failed", stableExecutionError(err))
	}
	if err := d.verifySource(ctx, source, request.BaselineCommit); err != nil {
		return fail("baseline_changed", stableExecutionError(err))
	}
	if _, err := d.runGit(ctx, source, nil, nil, "check-ref-format", "--branch", request.BaselineBranch); err != nil {
		return fail("delivery_policy_failed", "invalid baseline branch")
	}
	if _, err := d.runGit(ctx, source, nil, nil, "check-ref-format", "--branch", request.Branch); err != nil {
		return fail("delivery_policy_failed", "invalid delivery branch")
	}
	remoteOutput, err := d.runGit(ctx, source, nil, nil, "remote", "get-url", d.remote)
	if err != nil {
		return fail("remote_validation_failed", "approved Git remote is unavailable")
	}
	remoteURL := strings.TrimSpace(string(remoteOutput))
	if !validDeliveryRemote(remoteURL, d.allowLocalRemote) {
		return fail("remote_validation_failed", "approved Git remote is unsafe or unsupported")
	}
	parent, err := os.MkdirTemp(d.deliveryRoot, deliveryDirectoryPrefix+request.DeliveryID+"-")
	if err != nil {
		return fail("delivery_clone_failed", "create controlled delivery directory")
	}
	defer func() {
		cleanupErr := os.RemoveAll(parent)
		result.WorkingCopyRemoved = cleanupErr == nil
		if cleanupErr != nil {
			result.Status = DeliveryFailed
			result.ErrorCode = "delivery_cleanup_failed"
			result.ErrorMessage = "remove controlled delivery directory"
		}
	}()
	hooks := filepath.Join(parent, "hooks")
	if err := os.Mkdir(hooks, 0o700); err != nil {
		return fail("delivery_clone_failed", "prepare controlled Git hooks directory")
	}
	target := filepath.Join(parent, "repository")
	if _, err := d.runGit(ctx, d.deliveryRoot, nil, nil, "-c", "core.hooksPath="+hooks, "clone", "--no-hardlinks", "--no-tags", "--no-checkout", "--local", "--", source, target); err != nil {
		return fail("delivery_clone_failed", "create controlled delivery clone")
	}
	if _, err := d.runGit(ctx, target, nil, nil, "-c", "core.hooksPath="+hooks, "checkout", "-b", request.Branch, request.BaselineCommit, "--"); err != nil {
		return fail("branch_creation_failed", "create controlled delivery branch")
	}
	patchBytes := []byte(request.Patch)
	if _, err := d.runGit(ctx, target, nil, patchBytes, "apply", "--check", "--whitespace=error-all", "-"); err != nil {
		return fail("patch_check_failed", "verified patch no longer applies cleanly")
	}
	if _, err := d.runGit(ctx, target, nil, patchBytes, "apply", "--whitespace=error-all", "-"); err != nil {
		return fail("patch_apply_failed", "apply verified patch in delivery clone")
	}
	if err := scanChangedFiles(target, request.Files); err != nil {
		return fail("secret_scan_failed", stableExecutionError(err))
	}
	scanEvidence, scanErr := d.scanner.Scan(ctx, target)
	result.SecretScan = &scanEvidence
	if scanErr != nil {
		return fail("enterprise_secret_scan_failed", stableExecutionError(scanErr))
	}
	paths := make([]string, 0, len(request.Files)+2)
	paths = append(paths, "add", "--")
	for _, file := range request.Files {
		paths = append(paths, file.Path)
	}
	if _, err := d.runGit(ctx, target, nil, nil, paths...); err != nil {
		return fail("commit_failed", "stage approved files")
	}
	message := controlledCommitMessage(request.Summary, request.SessionID)
	if _, err := d.runGit(ctx, target, nil, nil,
		"-c", "user.name="+d.authorName, "-c", "user.email="+d.authorEmail,
		"commit", "--no-gpg-sign", "--no-verify", "-m", message,
	); err != nil {
		return fail("commit_failed", "create controlled commit")
	}
	commitOutput, err := d.runGit(ctx, target, nil, nil, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return fail("commit_failed", "resolve controlled commit")
	}
	result.Commit = strings.TrimSpace(string(commitOutput))
	pushRemoteURL := remoteURL
	if d.token != "" {
		pushRemoteURL, err = remoteWithUsername(remoteURL, d.username)
		if err != nil {
			return fail("credential_setup_failed", "approved Git remote does not support configured credentials")
		}
	}
	if _, err := d.runGit(ctx, target, nil, nil, "remote", "set-url", "origin", pushRemoteURL); err != nil {
		return fail("remote_validation_failed", "configure approved Git remote")
	}
	pushEnvironment, err := d.pushEnvironment(parent)
	if err != nil {
		return fail("credential_setup_failed", "prepare controlled Git credentials")
	}
	if _, err := d.runGit(ctx, target, pushEnvironment, nil, "push", "--porcelain", "--set-upstream", "origin", "HEAD:refs/heads/"+request.Branch); err != nil {
		return fail("push_failed", "push controlled branch without force")
	}
	result.BranchPushed = true
	prURL, err := d.publisher.Publish(ctx, target, PullRequestRequest{
		SessionID: request.SessionID, Project: request.Project, Head: request.Branch, Base: request.BaselineBranch,
		Title: controlledTitle(request.Summary), Body: controlledPullRequestBody(request),
	})
	if err != nil || !validPullRequestURL(prURL) {
		return fail("pull_request_failed", "create controlled pull request")
	}
	result.PullRequestURL = prURL
	if err := d.verifySource(ctx, source, request.BaselineCommit); err != nil {
		return fail("source_changed_during_delivery", stableExecutionError(err))
	}
	result.Status = DeliveryPassed
	result.ErrorCode = ""
	result.ErrorMessage = ""
	return result
}

func validPullRequestURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func (d *GitDeliverer) resolveSourceRepository(ctx context.Context, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return "", ErrInvalidDeliveryConfig
	}
	target, err := filepath.EvalSymlinks(filepath.Join(d.workspaceRoot, filepath.Clean(value)))
	if err != nil || !withinPath(d.workspaceRoot, target) {
		return "", ErrInvalidDeliveryConfig
	}
	output, err := d.runGit(ctx, target, nil, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrInvalidDeliveryConfig
	}
	top, err := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(output)))))
	if err != nil || !samePathValue(target, top) {
		return "", ErrInvalidDeliveryConfig
	}
	return top, nil
}

func (d *GitDeliverer) verifySource(ctx context.Context, source, baseline string) error {
	head, err := d.runGit(ctx, source, nil, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != baseline {
		return ErrBaselineChanged
	}
	status, err := d.runGit(ctx, source, nil, nil, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=all", "--no-renames")
	if err != nil || len(status) != 0 {
		return ErrDirtyWorkspace
	}
	return nil
}

func (d *GitDeliverer) runGit(ctx context.Context, directory string, extraEnvironment []string, stdin []byte, arguments ...string) ([]byte, error) {
	commandContext, cancel := context.WithTimeout(ctx, minDuration(d.timeout, 60*time.Second))
	defer cancel()
	gitArguments := append([]string{"-c", "core.fsmonitor=false", "-c", "maintenance.auto=false", "-c", "credential.useHttpPath=true"}, arguments...)
	command := exec.CommandContext(commandContext, d.gitBinary, gitArguments...)
	command.Dir = directory
	command.Env = append(deliveryEnvironment(), extraEnvironment...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	stdout := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	stderr := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	if stdout.exceeded || stderr.exceeded {
		return nil, errExecutionOutputLimit
	}
	if commandContext.Err() != nil {
		return nil, commandContext.Err()
	}
	if runErr != nil {
		return nil, runErr
	}
	return stdout.content.Bytes(), nil
}

func (d *GitDeliverer) pushEnvironment(parent string) ([]string, error) {
	if d.token == "" {
		return nil, nil
	}
	extension := ""
	content := "#!/bin/sh\nprintf '%s\\n' \"$EKBDA_GIT_TOKEN\"\n"
	if runtime.GOOS == "windows" {
		extension = ".cmd"
		content = "@echo off\r\npowershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command \"[Console]::Out.Write($env:EKBDA_GIT_TOKEN)\"\r\n"
	}
	path := filepath.Join(parent, "askpass"+extension)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		return nil, err
	}
	return []string{"GIT_ASKPASS=" + path, "GIT_ASKPASS_REQUIRE=force", "EKBDA_GIT_TOKEN=" + d.token}, nil
}

func deliveryEnvironment() []string {
	allowed := map[string]bool{
		"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true,
		"TMP": true, "TEMP": true, "LANG": true, "LC_ALL": true, "SSH_AUTH_SOCK": true, "SSH_AGENT_PID": true,
		"SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
	}
	environment := make([]string, 0, len(allowed)+6)
	for _, value := range os.Environ() {
		name, _, found := strings.Cut(value, "=")
		if found && allowed[strings.ToUpper(name)] {
			environment = append(environment, value)
		}
	}
	return append(environment,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0", "LC_ALL=C",
	)
}

func validDeliveryRemote(value string, allowLocal bool) bool {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
		return false
	}
	if allowLocal && filepath.IsAbs(value) {
		return true
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Scheme != "" {
		if parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "ssh") {
			return allowLocal && parsed.Scheme == "file"
		}
		return parsed.Host != ""
	}
	if strings.Contains(value, "@") && strings.Contains(value, ":") && !strings.Contains(value, " ") {
		return true
	}
	return allowLocal && filepath.IsAbs(value)
}

func remoteWithUsername(value, username string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || strings.TrimSpace(username) == "" {
		return "", ErrInvalidDeliveryConfig
	}
	parsed.User = url.User(username)
	return parsed.String(), nil
}

func controlledCommitMessage(summary, sessionID string) string {
	return controlledTitle(summary) + "\n\nEKBDA-Session: " + sessionID
}

func controlledTitle(summary string) string {
	summary = strings.TrimSpace(strings.Split(strings.ReplaceAll(summary, "\r", ""), "\n")[0])
	if len(summary) > 120 {
		summary = summary[:120]
	}
	if summary == "" {
		summary = "verified change"
	}
	return "EKBDA: " + summary
}

func controlledPullRequestBody(request DeliveryRequest) string {
	return fmt.Sprintf("Controlled EKBDA delivery.\n\nSession: %s\nBaseline: %s\nPatch: %s\n\nThis PR was created from an independently approved and strongly isolated verification. Merge and deployment require separate approval.", request.SessionID, request.BaselineCommit, request.PatchHash)
}
