package development

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

const (
	maxExecutionOutputBytes  = 1 << 20
	maxScannedFileBytes      = 1 << 20
	executionDirectoryPrefix = "ekbda-execution-"
)

var ErrInvalidExecutionConfig = errors.New("invalid controlled development execution configuration")

type RunRequest struct {
	ExecutionID    string
	SessionID      string
	Repository     string
	Project        string
	Technology     string
	BaselineCommit string
	PatchHash      string
	Patch          string
	Files          []FileChange
	Commands       []Command
	Actor          string
}

type Runner interface {
	Enabled() bool
	Run(context.Context, RunRequest) Execution
	RecoveryGracePeriod() time.Duration
	CleanupStale(context.Context, []string) error
}

type disabledRunner struct{}

func (disabledRunner) Enabled() bool                                { return false }
func (disabledRunner) Run(context.Context, RunRequest) Execution    { return Execution{} }
func (disabledRunner) RecoveryGracePeriod() time.Duration           { return 0 }
func (disabledRunner) CleanupStale(context.Context, []string) error { return nil }

type LocalRunner struct {
	enabled       bool
	workspaceRoot string
	executionRoot string
	gitBinary     string
	timeout       time.Duration
	standards     *standards.Service
	executionWS   *workspace.Service
	secretScanner SecretScanner
	container     *ContainerSandbox
	isolation     string
	networkPolicy string
}

func NewLocalRunner(enabled bool, workspaceRoot, executionRoot string, standardsService *standards.Service, timeout time.Duration) (*LocalRunner, error) {
	runner := &LocalRunner{
		enabled: enabled, standards: standardsService, timeout: timeout,
		isolation: "local_clone_no_hardlinks", networkPolicy: "offline_environment_best_effort",
	}
	if !enabled {
		return runner, nil
	}
	if standardsService == nil || timeout < time.Second {
		return nil, ErrInvalidExecutionConfig
	}
	resolvedWorkspace, err := resolveExistingDirectory(workspaceRoot)
	if err != nil {
		return nil, ErrInvalidExecutionConfig
	}
	resolvedExecution, err := resolveExistingDirectory(executionRoot)
	if err != nil || pathsOverlap(resolvedWorkspace, resolvedExecution) {
		return nil, ErrInvalidExecutionConfig
	}
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("%w: Git executable is required", ErrInvalidExecutionConfig)
	}
	executionWS, err := workspace.New(resolvedExecution, standardsService, workspace.NewMemoryStore())
	if err != nil {
		return nil, fmt.Errorf("%w: initialize isolated workspace reader: %v", ErrInvalidExecutionConfig, err)
	}
	runner.workspaceRoot = resolvedWorkspace
	runner.executionRoot = resolvedExecution
	runner.gitBinary = gitBinary
	runner.executionWS = executionWS
	return runner, nil
}

func NewContainerRunner(enabled bool, workspaceRoot, executionRoot string, standardsService *standards.Service, timeout time.Duration, scanner SecretScanner, containerConfig ContainerConfig) (*LocalRunner, error) {
	runner, err := NewLocalRunner(enabled, workspaceRoot, executionRoot, standardsService, timeout)
	if err != nil || !enabled {
		return runner, err
	}
	if scanner == nil {
		return nil, ErrInvalidExecutionConfig
	}
	container, err := NewContainerSandbox(containerConfig)
	if err != nil {
		return nil, err
	}
	runner.secretScanner = scanner
	runner.container = container
	runner.isolation = "non_privileged_container"
	runner.networkPolicy = "network_namespace_none"
	return runner, nil
}

func (r *LocalRunner) Enabled() bool { return r != nil && r.enabled }

func (r *LocalRunner) Profile() (string, string) {
	return r.isolation, r.networkPolicy
}

func (r *LocalRunner) RecoveryGracePeriod() time.Duration {
	if !r.Enabled() {
		return 0
	}
	return r.timeout + 30*time.Second
}

func (r *LocalRunner) CleanupStale(_ context.Context, executionIDs []string) error {
	if !r.Enabled() || len(executionIDs) == 0 {
		return nil
	}
	prefixes := make([]string, 0, len(executionIDs))
	for _, id := range executionIDs {
		if !validExecutionID(id) {
			return ErrInvalidExecutionConfig
		}
		prefixes = append(prefixes, executionDirectoryPrefix+id+"-")
	}
	entries, err := os.ReadDir(r.executionRoot)
	if err != nil {
		return fmt.Errorf("list controlled execution root: %w", err)
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
		target := filepath.Join(r.executionRoot, entry.Name())
		if !withinPath(r.executionRoot, target) {
			return ErrInvalidExecutionConfig
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove stale controlled execution directory: %w", err)
		}
	}
	return nil
}

func (r *LocalRunner) Run(ctx context.Context, request RunRequest) (result Execution) {
	overallContext, cancelOverall := context.WithTimeout(ctx, r.timeout)
	defer cancelOverall()
	ctx = overallContext
	started := time.Now().UTC()
	result = Execution{
		ID: request.ExecutionID, Status: ExecutionFailed, BaselineCommit: request.BaselineCommit,
		PatchHash: request.PatchHash, Isolation: r.isolation,
		NetworkPolicy: r.networkPolicy, ExecutedBy: request.Actor,
		StartedAt: started, Commands: []CommandEvidence{},
	}
	fail := func(code, message string) Execution {
		result.ErrorCode = code
		result.ErrorMessage = message
		return result
	}
	if !r.Enabled() {
		return fail("execution_disabled", ErrExecutionDisabled.Error())
	}
	source, err := r.resolveSourceRepository(ctx, request.Repository)
	if err != nil {
		return fail("source_validation_failed", stableExecutionError(err))
	}
	if err := r.verifySource(ctx, source, request.BaselineCommit); err != nil {
		return fail("baseline_changed", stableExecutionError(err))
	}
	if !validExecutionID(request.ExecutionID) {
		return fail("isolation_failed", "invalid execution identity")
	}
	parent, err := os.MkdirTemp(r.executionRoot, executionDirectoryPrefix+request.ExecutionID+"-")
	if err != nil {
		return fail("isolation_failed", "create isolated execution directory")
	}
	defer func() {
		cleanupErr := os.RemoveAll(parent)
		result.IsolatedCopyRemoved = cleanupErr == nil
		if cleanupErr != nil {
			result.Status = ExecutionFailed
			result.ErrorCode = "cleanup_failed"
			result.ErrorMessage = "remove isolated execution directory"
		}
		result.FinishedAt = time.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	}()
	hooks := filepath.Join(parent, "hooks")
	temp := filepath.Join(parent, "tmp")
	cache := filepath.Join(parent, "cache")
	for _, directory := range []string{hooks, temp, cache} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return fail("isolation_failed", "prepare isolated execution directories")
		}
	}
	target := filepath.Join(parent, "repository")
	if _, commandErr := r.runGit(ctx, r.executionRoot, hooks, nil, "clone", "--no-hardlinks", "--no-tags", "--no-checkout", "--local", "--", source, target); commandErr != nil {
		return fail("isolation_failed", "create isolated local clone")
	}
	if _, commandErr := r.runGit(ctx, target, hooks, nil, "checkout", "--detach", request.BaselineCommit, "--"); commandErr != nil {
		return fail("isolation_failed", "checkout approved baseline")
	}
	patchBytes := []byte(request.Patch)
	if _, commandErr := r.runGit(ctx, target, hooks, patchBytes, "apply", "--check", "--whitespace=error-all", "-"); commandErr != nil {
		return fail("patch_check_failed", "approved patch no longer applies cleanly")
	}
	if _, commandErr := r.runGit(ctx, target, hooks, patchBytes, "apply", "--whitespace=error-all", "-"); commandErr != nil {
		return fail("patch_apply_failed", "apply approved patch in isolated clone")
	}
	if err := scanChangedFiles(target, request.Files); err != nil {
		return fail("secret_scan_failed", stableExecutionError(err))
	}
	if r.secretScanner != nil {
		evidence, scanErr := r.secretScanner.Scan(ctx, target)
		result.SecretScan = &evidence
		if scanErr != nil {
			return fail("enterprise_secret_scan_failed", stableExecutionError(scanErr))
		}
	} else {
		result.SecretScan = &SecretScanEvidence{Scanner: "ekbda-defense-in-depth", Passed: true}
	}
	result.SecretScanPassed = result.SecretScan.Passed
	relativeTarget, err := filepath.Rel(r.executionRoot, target)
	if err != nil || strings.HasPrefix(relativeTarget, "..") {
		return fail("isolation_failed", "resolve isolated repository")
	}
	snapshot, err := r.executionWS.Read(ctx, filepath.ToSlash(relativeTarget))
	if err != nil {
		return fail("standards_gate_failed", "read isolated repository for standards validation")
	}
	report, err := r.standards.Validate(ctx, standards.ValidateInput{Project: request.Project, Technology: request.Technology, Files: snapshot.Files}, request.Actor)
	if err != nil {
		return fail("standards_gate_failed", stableExecutionError(err))
	}
	result.StandardsReportID = report.ID
	result.StandardsPassed = report.Passed
	if !report.Passed {
		return fail("standards_gate_failed", "blocking enterprise standard violation")
	}
	for _, requested := range request.Commands {
		command, found := commandCatalog[requested.ID]
		if !found {
			return fail("command_policy_failed", "approved command is no longer in the server catalog")
		}
		var evidence CommandEvidence
		var commandErr error
		if r.container != nil {
			evidence, commandErr = r.container.Run(ctx, target, command, r.timeout)
		} else {
			evidence, commandErr = r.runApprovedCommand(ctx, target, parent, command)
		}
		result.Commands = append(result.Commands, evidence)
		if commandErr != nil {
			code := "command_failed"
			if evidence.TimedOut {
				code = "command_timeout"
			}
			if errors.Is(commandErr, errExecutionOutputLimit) {
				code = "command_output_limit"
			}
			return fail(code, "approved command failed: "+command.ID)
		}
	}
	if err := r.verifySource(ctx, source, request.BaselineCommit); err != nil {
		return fail("source_changed_during_execution", stableExecutionError(err))
	}
	result.Status = ExecutionPassed
	result.ErrorCode = ""
	result.ErrorMessage = ""
	return result
}

func (r *LocalRunner) resolveSourceRepository(ctx context.Context, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return "", ErrInvalidExecutionConfig
	}
	cleaned := filepath.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", ErrInvalidExecutionConfig
	}
	target, err := filepath.EvalSymlinks(filepath.Join(r.workspaceRoot, cleaned))
	if err != nil || !withinPath(r.workspaceRoot, target) {
		return "", ErrInvalidExecutionConfig
	}
	output, commandErr := r.runGit(ctx, target, "", nil, "rev-parse", "--show-toplevel")
	if commandErr != nil {
		return "", ErrInvalidExecutionConfig
	}
	topLevel, err := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(output)))))
	if err != nil || !samePathValue(target, topLevel) || !withinPath(r.workspaceRoot, topLevel) {
		return "", ErrInvalidExecutionConfig
	}
	return topLevel, nil
}

func (r *LocalRunner) verifySource(ctx context.Context, source, baseline string) error {
	head, err := r.runGit(ctx, source, "", nil, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != baseline {
		return ErrBaselineChanged
	}
	status, err := r.runGit(ctx, source, "", nil, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=all", "--no-renames")
	if err != nil || len(status) != 0 {
		return ErrDirtyWorkspace
	}
	return nil
}

func (r *LocalRunner) runGit(ctx context.Context, directory, hooks string, stdin []byte, arguments ...string) ([]byte, error) {
	commandContext, cancel := context.WithTimeout(ctx, minDuration(r.timeout, 30*time.Second))
	defer cancel()
	gitArguments := []string{"-c", "core.fsmonitor=false", "-c", "maintenance.auto=false"}
	if hooks != "" {
		gitArguments = append(gitArguments, "-c", "core.hooksPath="+hooks, "-c", "init.templateDir=")
	}
	gitArguments = append(gitArguments, arguments...)
	command := exec.CommandContext(commandContext, r.gitBinary, gitArguments...)
	command.Dir = directory
	command.Env = isolatedExecutionEnvironment("", "")
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	stdout := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	stderr := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if stdout.exceeded || stderr.exceeded {
		return nil, errExecutionOutputLimit
	}
	if commandContext.Err() != nil {
		return nil, commandContext.Err()
	}
	if err != nil {
		return nil, err
	}
	return stdout.content.Bytes(), nil
}

func (r *LocalRunner) runApprovedCommand(ctx context.Context, target, parent string, command Command) (CommandEvidence, error) {
	started := time.Now()
	commandContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	executable, err := exec.LookPath(command.Executable)
	if err != nil {
		return CommandEvidence{ID: command.ID, ExitCode: -1}, err
	}
	process := exec.CommandContext(commandContext, executable, command.Arguments...)
	process.Dir = target
	process.Env = isolatedExecutionEnvironment(filepath.Join(parent, "cache"), filepath.Join(parent, "tmp"))
	stdout := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	stderr := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	process.Stdout = stdout
	process.Stderr = stderr
	runErr := process.Run()
	evidence := CommandEvidence{
		ID: command.ID, ExitCode: exitCode(runErr), TimedOut: errors.Is(commandContext.Err(), context.DeadlineExceeded),
		DurationMS: time.Since(started).Milliseconds(), StdoutBytes: stdout.total, StderrBytes: stderr.total,
		StdoutSHA256: stdout.digest(), StderrSHA256: stderr.digest(),
	}
	if stdout.exceeded || stderr.exceeded {
		return evidence, errExecutionOutputLimit
	}
	if commandContext.Err() != nil {
		return evidence, commandContext.Err()
	}
	return evidence, runErr
}

func scanChangedFiles(repository string, files []FileChange) error {
	for _, change := range files {
		if change.Operation == "delete" {
			continue
		}
		if sensitivePath(change.Path) {
			return ErrSensitiveContent
		}
		fullPath := filepath.Join(repository, filepath.FromSlash(change.Path))
		resolved, err := filepath.EvalSymlinks(fullPath)
		if err != nil || !withinPath(repository, resolved) {
			return ErrSensitiveContent
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxScannedFileBytes {
			return ErrSensitiveContent
		}
		content, err := os.ReadFile(resolved)
		if err != nil || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 || suspectedSecret(string(content)) {
			return ErrSensitiveContent
		}
	}
	return nil
}

var errExecutionOutputLimit = errors.New("execution output limit exceeded")

type cancelingEvidenceWriter struct {
	mu       sync.Mutex
	content  bytes.Buffer
	limit    int
	total    int
	exceeded bool
	cancel   context.CancelFunc
}

func (w *cancelingEvidenceWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.total += len(value)
	remaining := w.limit - w.content.Len()
	if remaining > 0 {
		writeValue := value
		if len(writeValue) > remaining {
			writeValue = writeValue[:remaining]
		}
		_, _ = w.content.Write(writeValue)
	}
	if w.total > w.limit && !w.exceeded {
		w.exceeded = true
		w.cancel()
	}
	return len(value), nil
}

func (w *cancelingEvidenceWriter) digest() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	digest := sha256.Sum256(w.content.Bytes())
	return hex.EncodeToString(digest[:])
}

func isolatedExecutionEnvironment(cacheRoot, tempRoot string) []string {
	allowed := map[string]bool{
		"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true,
		"OS": true, "PROCESSOR_ARCHITECTURE": true, "PROCESSOR_IDENTIFIER": true, "NUMBER_OF_PROCESSORS": true,
		"LANG": true, "LC_ALL": true,
	}
	environment := make([]string, 0, len(allowed)+16)
	for _, value := range os.Environ() {
		name, _, found := strings.Cut(value, "=")
		if found && allowed[strings.ToUpper(name)] {
			environment = append(environment, value)
		}
	}
	environment = append(environment,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0",
		"GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOENV=off", "GOWORK=off", "CGO_ENABLED=0",
		"HTTP_PROXY=http://127.0.0.1:9", "HTTPS_PROXY=http://127.0.0.1:9", "ALL_PROXY=http://127.0.0.1:9", "NO_PROXY=",
		"LC_ALL=C",
	)
	if cacheRoot != "" {
		environment = append(environment, "GOCACHE="+filepath.Join(cacheRoot, "go-build"), "GOMODCACHE="+filepath.Join(cacheRoot, "go-mod"))
	}
	if tempRoot != "" {
		environment = append(environment, "GOTMPDIR="+tempRoot, "TMP="+tempRoot, "TEMP="+tempRoot)
	}
	return environment
}

func resolveExistingDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidExecutionConfig
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", ErrInvalidExecutionConfig
	}
	return filepath.Clean(resolved), nil
}

func pathsOverlap(first, second string) bool {
	return withinPath(first, second) || withinPath(second, first)
}

func withinPath(base, target string) bool {
	relative, err := filepath.Rel(base, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePathValue(first, second string) bool {
	relative, err := filepath.Rel(first, second)
	return err == nil && relative == "."
}

func validExecutionID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func minDuration(first, second time.Duration) time.Duration {
	if first < second {
		return first
	}
	return second
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

func stableExecutionError(err error) string {
	switch {
	case errors.Is(err, ErrBaselineChanged):
		return ErrBaselineChanged.Error()
	case errors.Is(err, ErrDirtyWorkspace):
		return ErrDirtyWorkspace.Error()
	case errors.Is(err, ErrSensitiveContent):
		return ErrSensitiveContent.Error()
	case errors.Is(err, ErrEnterpriseSecretScan):
		return ErrEnterpriseSecretScan.Error()
	case errors.Is(err, standards.ErrNoApplicablePackages):
		return standards.ErrNoApplicablePackages.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return "execution timed out"
	default:
		return "controlled execution step failed"
	}
}
