package development

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	containerImagePattern = regexp.MustCompile(`^[A-Za-z0-9._/:@-]+@sha256:[0-9a-f]{64}$`)
	containerLimitPattern = regexp.MustCompile(`^[1-9][0-9]*(?:[kKmMgG])?$`)
)

type ContainerConfig struct {
	Binary          string
	Image           string
	CPUs            string
	Memory          string
	PIDs            int
	WritableTmpSize string
	User            string
	GoModCache      string
}

type ContainerSandbox struct {
	binary          string
	image           string
	cpus            string
	memory          string
	pids            int
	writableTmpSize string
	user            string
	goModCache      string
}

func NewContainerSandbox(config ContainerConfig) (*ContainerSandbox, error) {
	config.Binary = strings.TrimSpace(config.Binary)
	if config.Binary == "" {
		config.Binary = "docker"
	}
	if !containerImagePattern.MatchString(config.Image) || !validPositiveDecimal(config.CPUs) || !containerLimitPattern.MatchString(config.Memory) || config.PIDs < 1 || config.PIDs > 4096 || !containerLimitPattern.MatchString(config.WritableTmpSize) || !validContainerUser(config.User) {
		return nil, ErrInvalidExecutionConfig
	}
	binary, err := exec.LookPath(config.Binary)
	if err != nil {
		return nil, fmt.Errorf("%w: container runtime executable is required", ErrInvalidExecutionConfig)
	}
	cache := ""
	if strings.TrimSpace(config.GoModCache) != "" {
		cache, err = resolveExistingDirectory(config.GoModCache)
		if err != nil {
			return nil, ErrInvalidExecutionConfig
		}
	}
	return &ContainerSandbox{
		binary: binary, image: config.Image, cpus: config.CPUs, memory: config.Memory,
		pids: config.PIDs, writableTmpSize: config.WritableTmpSize, user: config.User, goModCache: cache,
	}, nil
}

func (s *ContainerSandbox) Run(ctx context.Context, repository string, command Command, timeout time.Duration) (CommandEvidence, error) {
	started := time.Now()
	if strings.ContainsAny(repository, ",\r\n") {
		return CommandEvidence{ID: command.ID, ExitCode: -1}, ErrInvalidExecutionConfig
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	arguments := s.arguments(repository, command)
	process := exec.CommandContext(commandContext, s.binary, arguments...)
	process.Env = isolatedExecutionEnvironment("", "")
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

func (s *ContainerSandbox) arguments(repository string, command Command) []string {
	mount := "type=bind,source=" + filepath.Clean(repository) + ",target=/workspace,readonly"
	arguments := []string{
		"run", "--rm", "--pull=never", "--network=none", "--ipc=none", "--read-only", "--cap-drop=ALL",
		"--security-opt=no-new-privileges", "--pids-limit=" + strconv.Itoa(s.pids),
		"--memory=" + s.memory, "--cpus=" + s.cpus, "--user=" + s.user,
		"--workdir=/workspace", "--mount", mount,
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=" + s.writableTmpSize,
		"--env", "HOME=/tmp", "--env", "TMPDIR=/tmp", "--env", "GOCACHE=/tmp/go-build",
		"--env", "GOMODCACHE=/tmp/go-mod", "--env", "GOPROXY=off", "--env", "GOSUMDB=off",
		"--env", "GOTOOLCHAIN=local", "--env", "GOENV=off", "--env", "GOWORK=off", "--env", "CGO_ENABLED=0",
		"--env", "GIT_CONFIG_NOSYSTEM=1", "--env", "GIT_CONFIG_GLOBAL=/dev/null",
		"--env", "GIT_OPTIONAL_LOCKS=0", "--env", "GIT_TERMINAL_PROMPT=0", "--env", "LC_ALL=C",
	}
	if s.goModCache != "" {
		arguments = append(arguments,
			"--mount", "type=bind,source="+filepath.Clean(s.goModCache)+",target=/opt/ekbda/go-mod,readonly",
			"--env", "GOMODCACHE=/opt/ekbda/go-mod",
		)
	}
	arguments = append(arguments, "--entrypoint", command.Executable, s.image)
	return append(arguments, command.Arguments...)
}

func validPositiveDecimal(value string) bool {
	number, err := strconv.ParseFloat(value, 64)
	return err == nil && number > 0 && number <= 64
}

func validContainerUser(value string) bool {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 1 || number > 65535 {
			return false
		}
	}
	return true
}
