package development

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

var ErrInvalidPullRequestConfig = errors.New("invalid controlled pull request provider configuration")

type GitHubCLIPublisher struct {
	binary  string
	timeout time.Duration
}

func NewGitHubCLIPublisher(binary string, timeout time.Duration) (*GitHubCLIPublisher, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "gh"
	}
	if timeout < time.Second {
		return nil, ErrInvalidPullRequestConfig
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, ErrInvalidPullRequestConfig
	}
	return &GitHubCLIPublisher{binary: resolved, timeout: timeout}, nil
}

func (p *GitHubCLIPublisher) Publish(ctx context.Context, repository string, request PullRequestRequest) (string, error) {
	commandContext, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, p.binary,
		"pr", "create", "--head", request.Head, "--base", request.Base,
		"--title", request.Title, "--body", request.Body,
	)
	command.Dir = repository
	command.Env = pullRequestEnvironment()
	stdout := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	stderr := &cancelingEvidenceWriter{limit: maxExecutionOutputBytes, cancel: cancel}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil || commandContext.Err() != nil || stdout.exceeded || stderr.exceeded {
		return "", ErrInvalidPullRequestConfig
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout.content.String()), "\n") {
		candidate := strings.TrimSpace(line)
		parsed, err := url.Parse(candidate)
		if err == nil && parsed.Scheme == "https" && parsed.Host != "" {
			return candidate, nil
		}
	}
	return "", ErrInvalidPullRequestConfig
}

func pullRequestEnvironment() []string {
	allowed := map[string]bool{
		"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true,
		"TMP": true, "TEMP": true, "LANG": true, "LC_ALL": true,
		"GH_TOKEN": true, "GH_ENTERPRISE_TOKEN": true, "GH_HOST": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
		"SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
	}
	environment := make([]string, 0, len(allowed)+1)
	for _, value := range os.Environ() {
		name, _, found := strings.Cut(value, "=")
		if found && allowed[strings.ToUpper(name)] {
			environment = append(environment, value)
		}
	}
	return append(environment, "LC_ALL=C")
}
