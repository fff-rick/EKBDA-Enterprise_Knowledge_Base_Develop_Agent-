package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ekbda/internal/standards"
)

var (
	ErrDisabled           = errors.New("Git workspace access is not configured")
	ErrInvalidRepository  = errors.New("repository must be a Git root inside the configured workspace root")
	ErrRepositoryTooLarge = errors.New("repository exceeds the validation file or content limit")
	ErrRepositoryChanged  = errors.New("Git repository changed during read")
)

const (
	maxRepositoryFiles = 1000
	maxFileBytes       = 1 << 20
	maxRepositoryBytes = 5 << 20
	maxGitOutputBytes  = 4 << 20
	maxGitErrorBytes   = 16 << 10
	gitCommandTimeout  = 15 * time.Second
)

var commitPattern = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)

type Service struct {
	root      string
	gitBinary string
	standards *standards.Service
	store     Store
}

func New(root string, standardsService *standards.Service, store Store) (*Service, error) {
	service := &Service{standards: standardsService, store: store}
	root = strings.TrimSpace(root)
	if root == "" {
		return service, nil
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("workspace root must be an existing directory")
	}
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Git executable is required: %w", err)
	}
	service.root = filepath.Clean(resolved)
	service.gitBinary = gitBinary
	return service, nil
}

func (s *Service) Validate(ctx context.Context, input ValidateInput, validatedBy string) (Result, error) {
	content, err := s.Read(ctx, input.Repository)
	if err != nil {
		return Result{}, err
	}
	standardReport, err := s.standards.Validate(ctx, standards.ValidateInput{
		Project: input.Project, Technology: input.Technology, Files: content.Files,
	}, validatedBy)
	if err != nil {
		return Result{}, err
	}
	snapshot := Snapshot{
		ID: newID(), Repository: content.Repository,
		Project: standardReport.Project, Technology: standardReport.Technology,
		HeadCommit: content.HeadCommit, Branch: content.Branch,
		Dirty: content.Dirty, FileCount: len(content.Files), TrackedCount: content.TrackedCount,
		UntrackedCount: content.UntrackedCount, BinaryCount: content.BinaryCount,
		ChangedCount: len(content.Changes), Changes: content.Changes, InputHash: standardReport.InputHash,
		StandardsReportID: standardReport.ID, Passed: standardReport.Passed,
		ValidatedBy: strings.TrimSpace(validatedBy), CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Save(ctx, snapshot); err != nil {
		return Result{}, fmt.Errorf("save workspace validation snapshot: %w", err)
	}
	return Result{Repository: snapshot, Standards: standardReport}, nil
}

func (s *Service) Read(ctx context.Context, repositoryValue string) (ContentSnapshot, error) {
	if s.root == "" {
		return ContentSnapshot{}, ErrDisabled
	}
	repository, relativeRepository, err := s.resolveRepository(ctx, repositoryValue)
	if err != nil {
		return ContentSnapshot{}, err
	}
	headCommit := s.optionalGitValue(ctx, repository, "rev-parse", "--verify", "HEAD")
	tracked, err := s.listPaths(ctx, repository, true)
	if err != nil {
		return ContentSnapshot{}, err
	}
	untracked, err := s.listPaths(ctx, repository, false)
	if err != nil {
		return ContentSnapshot{}, err
	}
	if len(tracked)+len(untracked) > maxRepositoryFiles {
		return ContentSnapshot{}, ErrRepositoryTooLarge
	}
	files, trackedCount, untrackedCount, binaryCount, err := s.readFiles(repository, tracked, untracked)
	if err != nil {
		return ContentSnapshot{}, err
	}
	changes, err := s.status(ctx, repository)
	if err != nil {
		return ContentSnapshot{}, err
	}
	if headCommit != s.optionalGitValue(ctx, repository, "rev-parse", "--verify", "HEAD") {
		return ContentSnapshot{}, ErrRepositoryChanged
	}
	return ContentSnapshot{
		Repository: relativeRepository,
		HeadCommit: headCommit,
		Branch:     s.optionalGitValue(ctx, repository, "symbolic-ref", "--quiet", "--short", "HEAD"),
		Dirty:      len(changes) > 0, TrackedCount: trackedCount, UntrackedCount: untrackedCount,
		BinaryCount: binaryCount, Files: files, Changes: changes,
	}, nil
}

func (s *Service) Inspect(ctx context.Context, repositoryValue string) (RepositoryState, error) {
	if s.root == "" {
		return RepositoryState{}, ErrDisabled
	}
	repository, relativeRepository, err := s.resolveRepository(ctx, repositoryValue)
	if err != nil {
		return RepositoryState{}, err
	}
	headCommit := s.optionalGitValue(ctx, repository, "rev-parse", "--verify", "HEAD")
	tracked, err := s.listPaths(ctx, repository, true)
	if err != nil {
		return RepositoryState{}, err
	}
	untracked, err := s.listPaths(ctx, repository, false)
	if err != nil {
		return RepositoryState{}, err
	}
	if len(tracked)+len(untracked) > maxRepositoryFiles {
		return RepositoryState{}, ErrRepositoryTooLarge
	}
	changes, err := s.status(ctx, repository)
	if err != nil {
		return RepositoryState{}, err
	}
	if headCommit != s.optionalGitValue(ctx, repository, "rev-parse", "--verify", "HEAD") {
		return RepositoryState{}, ErrRepositoryChanged
	}
	return RepositoryState{
		Repository: relativeRepository, HeadCommit: headCommit,
		Branch: s.optionalGitValue(ctx, repository, "symbolic-ref", "--quiet", "--short", "HEAD"),
		Dirty:  len(changes) > 0, TrackedCount: len(tracked), UntrackedCount: len(untracked), Changes: changes,
	}, nil
}

func (s *Service) CommitDiff(ctx context.Context, repositoryValue, fromCommit, toCommit string) ([]CommitChange, error) {
	if s.root == "" {
		return nil, ErrDisabled
	}
	if !commitPattern.MatchString(fromCommit) || !commitPattern.MatchString(toCommit) {
		return nil, ErrInvalidRepository
	}
	repository, _, err := s.resolveRepository(ctx, repositoryValue)
	if err != nil {
		return nil, err
	}
	output, err := s.runGit(ctx, repository, "diff", "--name-status", "-z", "--no-renames", fromCommit, toCommit, "--")
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	changes := make([]CommitChange, 0, len(parts)/2)
	for index := 0; index < len(parts); {
		if len(parts[index]) == 0 {
			index++
			continue
		}
		if index+1 >= len(parts) || !utf8.Valid(parts[index]) || !utf8.Valid(parts[index+1]) {
			return nil, fmt.Errorf("parse Git commit diff: invalid name-status record")
		}
		status := string(parts[index])
		if len(status) != 1 || !strings.Contains("AMDTCUXB", status) {
			return nil, fmt.Errorf("parse Git commit diff: unsupported status")
		}
		filePath, err := normalizeGitPath(string(parts[index+1]))
		if err != nil {
			return nil, err
		}
		changes = append(changes, CommitChange{Path: filePath, Status: status})
		index += 2
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func (s *Service) Get(ctx context.Context, id string) (Result, error) {
	snapshot, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Result{}, err
	}
	report, err := s.standards.GetReport(ctx, snapshot.StandardsReportID)
	if err != nil {
		return Result{}, fmt.Errorf("load workspace standards report: %w", err)
	}
	return Result{Repository: snapshot, Standards: report}, nil
}

func (s *Service) List(ctx context.Context, project string, limit int) ([]Snapshot, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.List(ctx, strings.ToLower(strings.TrimSpace(project)), limit)
}

func (s *Service) resolveRepository(ctx context.Context, value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return "", "", ErrInvalidRepository
	}
	cleaned := filepath.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", "", ErrInvalidRepository
	}
	target, err := filepath.EvalSymlinks(filepath.Join(s.root, cleaned))
	if err != nil || !within(s.root, target) {
		return "", "", ErrInvalidRepository
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return "", "", ErrInvalidRepository
	}
	topLevelOutput, err := s.runGit(ctx, target, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", ErrInvalidRepository
	}
	topLevel := strings.TrimSpace(string(topLevelOutput))
	resolvedTopLevel, err := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(topLevel)))
	if err != nil || !within(s.root, resolvedTopLevel) || !samePath(target, resolvedTopLevel) {
		return "", "", ErrInvalidRepository
	}
	relative, err := filepath.Rel(s.root, resolvedTopLevel)
	if err != nil {
		return "", "", ErrInvalidRepository
	}
	return resolvedTopLevel, filepath.ToSlash(relative), nil
}

func (s *Service) listPaths(ctx context.Context, repository string, tracked bool) ([]string, error) {
	arguments := []string{"ls-files", "-z", "--deduplicate"}
	if tracked {
		arguments = append(arguments, "--cached")
	} else {
		arguments = append(arguments, "--others", "--exclude-standard")
	}
	output, err := s.runGit(ctx, repository, arguments...)
	if err != nil {
		return nil, err
	}
	return parseNULPaths(output)
}

func (s *Service) readFiles(repository string, tracked, untracked []string) ([]standards.File, int, int, int, error) {
	files := make([]standards.File, 0, len(tracked)+len(untracked))
	totalBytes := 0
	binaryCount := 0
	trackedCount := 0
	untrackedCount := 0
	seen := make(map[string]struct{}, len(tracked)+len(untracked))
	read := func(filePath string, isTracked bool) error {
		if _, exists := seen[filePath]; exists {
			return nil
		}
		seen[filePath] = struct{}{}
		fullPath := filepath.Join(repository, filepath.FromSlash(filePath))
		resolvedPath, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return ErrInvalidRepository
		}
		if !within(repository, resolvedPath) {
			return ErrInvalidRepository
		}
		info, err := os.Stat(resolvedPath)
		if err != nil {
			return ErrInvalidRepository
		}
		if info.IsDir() {
			files = append(files, standards.File{Path: filePath})
		} else {
			if !info.Mode().IsRegular() || info.Size() > maxFileBytes {
				return ErrRepositoryTooLarge
			}
			content, err := os.ReadFile(resolvedPath)
			if err != nil || len(content) > maxFileBytes {
				return ErrRepositoryTooLarge
			}
			text := ""
			if utf8.Valid(content) && bytes.IndexByte(content, 0) < 0 {
				text = string(content)
				totalBytes += len(content)
				if totalBytes > maxRepositoryBytes {
					return ErrRepositoryTooLarge
				}
			} else {
				binaryCount++
			}
			files = append(files, standards.File{Path: filePath, Content: text})
		}
		if isTracked {
			trackedCount++
		} else {
			untrackedCount++
		}
		return nil
	}
	for _, filePath := range tracked {
		if err := read(filePath, true); err != nil {
			return nil, 0, 0, 0, err
		}
	}
	for _, filePath := range untracked {
		if err := read(filePath, false); err != nil {
			return nil, 0, 0, 0, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, trackedCount, untrackedCount, binaryCount, nil
}

func (s *Service) status(ctx context.Context, repository string) ([]Change, error) {
	output, err := s.runGit(ctx, repository, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all", "--no-renames")
	if err != nil {
		return nil, err
	}
	records := bytes.Split(output, []byte{0})
	changes := make([]Change, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		if len(record) < 4 || record[2] != ' ' || !utf8.Valid(record[3:]) {
			return nil, fmt.Errorf("parse Git status: invalid porcelain record")
		}
		filePath, err := normalizeGitPath(string(record[3:]))
		if err != nil {
			return nil, err
		}
		changes = append(changes, Change{
			Path: filePath, IndexStatus: string(record[0]), WorktreeStatus: string(record[1]),
		})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func (s *Service) optionalGitValue(ctx context.Context, repository string, arguments ...string) string {
	output, err := s.runGit(ctx, repository, arguments...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (s *Service) runGit(ctx context.Context, repository string, arguments ...string) ([]byte, error) {
	commandContext, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	gitArguments := []string{"-c", "core.fsmonitor=false", "-c", "maintenance.auto=false"}
	gitArguments = append(gitArguments, arguments...)
	command := exec.CommandContext(commandContext, s.gitBinary, gitArguments...)
	command.Dir = repository
	command.Env = isolatedGitEnvironment()
	stdout := &cappedBuffer{limit: maxGitOutputBytes}
	stderr := &cappedBuffer{limit: maxGitErrorBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if commandContext.Err() != nil {
		return nil, fmt.Errorf("Git command timed out: %w", commandContext.Err())
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, ErrRepositoryTooLarge
	}
	if err != nil {
		return nil, fmt.Errorf("Git command failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func parseNULPaths(output []byte) ([]string, error) {
	parts := bytes.Split(output, []byte{0})
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		if !utf8.Valid(part) {
			return nil, ErrInvalidRepository
		}
		filePath, err := normalizeGitPath(string(part))
		if err != nil {
			return nil, err
		}
		if _, exists := seen[filePath]; exists {
			continue
		}
		seen[filePath] = struct{}{}
		result = append(result, filePath)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeGitPath(value string) (string, error) {
	value = filepath.ToSlash(value)
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if value == "" || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", ErrInvalidRepository
	}
	return filepath.ToSlash(cleaned), nil
}

func isolatedGitEnvironment() []string {
	environment := make([]string, 0)
	for _, value := range os.Environ() {
		name, _, found := strings.Cut(value, "=")
		if found && strings.HasPrefix(strings.ToUpper(name), "GIT_") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	)
}

func within(base, target string) bool {
	relative, err := filepath.Rel(base, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(first, second string) bool {
	relative, err := filepath.Rel(first, second)
	return err == nil && relative == "."
}

type cappedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	length := len(value)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.Buffer.Write(value)
	}
	if length > remaining {
		b.exceeded = true
	}
	return length, nil
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("workspace-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
