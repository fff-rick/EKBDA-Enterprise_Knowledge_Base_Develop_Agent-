package development

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidSecretScannerConfig = errors.New("invalid enterprise secret scanner configuration")
	ErrEnterpriseSecretScan       = errors.New("enterprise secret scanner rejected or failed to scan repository")
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

type SecretScanner interface {
	Name() string
	Scan(context.Context, string) (SecretScanEvidence, error)
}

type ExternalSecretScanner struct {
	name        string
	binary      string
	arguments   []string
	environment []string
	timeout     time.Duration
}

func NewExternalSecretScanner(name, binary string, arguments, environmentAllowlist []string, timeout time.Duration) (*ExternalSecretScanner, error) {
	name = strings.TrimSpace(name)
	binary = strings.TrimSpace(binary)
	if name == "" || binary == "" || timeout < time.Second || countPlaceholder(arguments, "{repository}") != 1 {
		return nil, ErrInvalidSecretScannerConfig
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, ErrInvalidSecretScannerConfig
	}
	allowed := make([]string, 0, len(environmentAllowlist))
	seen := make(map[string]struct{}, len(environmentAllowlist))
	for _, value := range environmentAllowlist {
		value = strings.TrimSpace(value)
		upper := strings.ToUpper(value)
		if value == "" || !environmentNamePattern.MatchString(value) || strings.HasPrefix(upper, "GIT_") || upper == "PATH" || upper == "HOME" {
			return nil, ErrInvalidSecretScannerConfig
		}
		if _, found := seen[upper]; found {
			continue
		}
		seen[upper] = struct{}{}
		allowed = append(allowed, value)
	}
	return &ExternalSecretScanner{name: name, binary: resolved, arguments: append([]string(nil), arguments...), environment: allowed, timeout: timeout}, nil
}

func (s *ExternalSecretScanner) Name() string { return s.name }

func (s *ExternalSecretScanner) Scan(ctx context.Context, repository string) (SecretScanEvidence, error) {
	started := time.Now()
	evidence := SecretScanEvidence{Scanner: s.name}
	resolved, err := resolveExistingDirectory(repository)
	if err != nil {
		return evidence, ErrEnterpriseSecretScan
	}
	arguments := make([]string, len(s.arguments))
	for index, value := range s.arguments {
		arguments[index] = strings.ReplaceAll(value, "{repository}", filepath.Clean(resolved))
	}
	commandContext, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, s.binary, arguments...)
	command.Dir = resolved
	command.Env = scannerEnvironment(s.environment)
	output := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	command.Stdout = output
	command.Stderr = output
	runErr := command.Run()
	evidence.DurationMS = time.Since(started).Milliseconds()
	evidence.OutputBytes = output.total
	evidence.OutputSHA256 = output.digest()
	if output.exceeded || commandContext.Err() != nil || runErr != nil {
		return evidence, ErrEnterpriseSecretScan
	}
	evidence.Passed = true
	return evidence, nil
}

func countPlaceholder(values []string, placeholder string) int {
	count := 0
	for _, value := range values {
		count += strings.Count(value, placeholder)
	}
	return count
}

func scannerEnvironment(allowlist []string) []string {
	baseAllowed := map[string]bool{
		"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true,
		"LANG": true, "LC_ALL": true, "TMP": true, "TEMP": true,
	}
	for _, value := range allowlist {
		baseAllowed[strings.ToUpper(value)] = true
	}
	environment := make([]string, 0, len(baseAllowed)+1)
	for _, value := range os.Environ() {
		name, _, found := strings.Cut(value, "=")
		if found && baseAllowed[strings.ToUpper(name)] {
			environment = append(environment, value)
		}
	}
	return append(environment, "LC_ALL=C")
}
