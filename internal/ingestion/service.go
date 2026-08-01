package ingestion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"ekbda/internal/knowledge"
)

const maxFileSize = 2 << 20

var (
	ErrDisabled    = errors.New("knowledge file import is disabled")
	ErrInvalidPath = errors.New("invalid import path")
)

type Input struct {
	Path           string                   `json:"path"`
	Project        string                   `json:"project"`
	BusinessDomain string                   `json:"business_domain"`
	Classification knowledge.Classification `json:"classification"`
	AllowedRoles   []string                 `json:"allowed_roles"`
}

type FileResult struct {
	Path       string                 `json:"path"`
	Action     knowledge.ImportAction `json:"action,omitempty"`
	DocumentID string                 `json:"document_id,omitempty"`
	Version    int                    `json:"version,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

type Report struct {
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	Root        string       `json:"root"`
	Project     string       `json:"project"`
	Scanned     int          `json:"scanned"`
	Created     int          `json:"created"`
	Updated     int          `json:"updated"`
	Skipped     int          `json:"skipped"`
	Deleted     int          `json:"deleted"`
	Failed      int          `json:"failed"`
	Error       string       `json:"error,omitempty"`
	Files       []FileResult `json:"files"`
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt time.Time    `json:"completed_at"`
}

type Service struct {
	root      string
	knowledge *knowledge.Service
	jobs      JobStore
}

func New(root string, knowledgeService *knowledge.Service, jobStore JobStore) *Service {
	if jobStore == nil {
		jobStore = NewMemoryJobStore()
	}
	return &Service{root: strings.TrimSpace(root), knowledge: knowledgeService, jobs: jobStore}
}

func (s *Service) Import(ctx context.Context, input Input) (Report, error) {
	report, base, target, err := s.prepare(input)
	if err != nil {
		return report, err
	}
	if err := s.jobs.Create(ctx, report); err != nil {
		return report, err
	}
	return s.execute(ctx, report, base, target, input)
}

func (s *Service) Start(ctx context.Context, input Input) (Report, error) {
	report, base, target, err := s.prepare(input)
	if err != nil {
		return report, err
	}
	if err := s.jobs.Create(ctx, report); err != nil {
		return report, err
	}
	go func() {
		jobContext, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_, _ = s.execute(jobContext, report, base, target, input)
	}()
	return report, nil
}

func (s *Service) Get(ctx context.Context, id string) (Report, error) {
	return s.jobs.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) prepare(input Input) (Report, string, string, error) {
	report := Report{
		ID:        newID(),
		Status:    "pending",
		Project:   strings.TrimSpace(input.Project),
		Files:     make([]FileResult, 0),
		StartedAt: time.Now().UTC(),
	}
	if s.root == "" {
		return report, "", "", ErrDisabled
	}
	if report.Project == "" {
		return report, "", "", fmt.Errorf("project is required")
	}

	base, target, err := resolveTarget(s.root, input.Path)
	if err != nil {
		return report, "", "", err
	}
	report.Root = target
	return report, base, target, nil
}

func (s *Service) execute(ctx context.Context, report Report, base, target string, input Input) (Report, error) {
	report.Status = "running"
	if err := s.jobs.Update(ctx, report, nil); err != nil {
		return report, err
	}
	seen := make(map[string]bool)
	err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return s.recordResult(ctx, &report, FileResult{Path: path, Error: walkErr.Error()})
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != target && ignoredDirectories[strings.ToLower(entry.Name())] {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !supportedFile(path) {
			return nil
		}
		seen[sourceURI(base, path)] = true
		report.Scanned++
		result := s.importFile(ctx, base, path, input)
		return s.recordResult(ctx, &report, result)
	})
	if err == nil {
		info, statErr := os.Stat(target)
		if statErr != nil {
			err = statErr
		} else if info.IsDir() {
			prefix, prefixErr := sourcePrefix(base, target)
			if prefixErr != nil {
				err = prefixErr
			} else {
				deleted, deleteErr := s.knowledge.MarkMissingSources(ctx, report.Project, prefix, seen)
				if deleteErr != nil {
					err = deleteErr
				} else {
					for _, document := range deleted {
						result := FileResult{
							Path:       strings.TrimPrefix(document.SourceURI, "file:///"),
							Action:     knowledge.ImportActionDeleted,
							DocumentID: document.ID,
							Version:    document.Version,
						}
						if updateErr := s.recordResult(ctx, &report, result); updateErr != nil {
							err = updateErr
							break
						}
					}
				}
			}
		}
	}
	report.CompletedAt = time.Now().UTC()
	if err != nil {
		report.Status = "failed"
		report.Error = err.Error()
		_ = s.jobs.Update(context.Background(), report, nil)
		return report, err
	}
	if report.Failed > 0 {
		report.Status = "completed_with_errors"
	} else {
		report.Status = "completed"
	}
	if err := s.jobs.Update(ctx, report, nil); err != nil {
		return report, err
	}
	return report, nil
}

func (s *Service) recordResult(ctx context.Context, report *Report, result FileResult) error {
	report.Files = append(report.Files, result)
	switch result.Action {
	case knowledge.ImportActionCreated:
		report.Created++
	case knowledge.ImportActionUpdated:
		report.Updated++
	case knowledge.ImportActionSkipped:
		report.Skipped++
	case knowledge.ImportActionDeleted:
		report.Deleted++
	default:
		report.Failed++
	}
	return s.jobs.Update(ctx, *report, &result)
}

func (s *Service) importFile(ctx context.Context, base, path string, input Input) FileResult {
	relativePath, err := filepath.Rel(base, path)
	if err != nil {
		return FileResult{Path: path, Error: err.Error()}
	}
	displayPath := filepath.ToSlash(relativePath)
	info, err := os.Stat(path)
	if err != nil {
		return FileResult{Path: displayPath, Error: err.Error()}
	}
	if info.Size() > maxFileSize {
		return FileResult{Path: displayPath, Error: "file exceeds 2 MiB limit"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return FileResult{Path: displayPath, Error: err.Error()}
	}
	if !utf8.Valid(data) {
		return FileResult{Path: displayPath, Error: "file is not valid UTF-8 text"}
	}
	hash := sha256.Sum256(data)
	document, action, err := s.knowledge.Import(ctx, knowledge.ImportDocumentInput{
		CreateDocumentInput: knowledge.CreateDocumentInput{
			Title:          displayPath,
			Content:        string(data),
			SourceURI:      sourceURI(base, path),
			BusinessDomain: input.BusinessDomain,
			Project:        input.Project,
			Classification: input.Classification,
			AllowedRoles:   input.AllowedRoles,
		},
		ContentHash: hex.EncodeToString(hash[:]),
	})
	if err != nil {
		return FileResult{Path: displayPath, Error: err.Error()}
	}
	return FileResult{
		Path:       displayPath,
		Action:     action,
		DocumentID: document.ID,
		Version:    document.Version,
	}
}

func resolveTarget(root, requestedPath string) (string, string, error) {
	if filepath.IsAbs(requestedPath) {
		return "", "", ErrInvalidPath
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve import root: %w", err)
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", "", fmt.Errorf("resolve import root: %w", err)
	}
	if strings.TrimSpace(requestedPath) == "" {
		requestedPath = "."
	}
	target, err := filepath.Abs(filepath.Join(base, requestedPath))
	if err != nil {
		return "", "", ErrInvalidPath
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", fmt.Errorf("resolve import path: %w", err)
	}
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", ErrInvalidPath
	}
	return base, target, nil
}

func supportedFile(path string) bool {
	return supportedExtensions[strings.ToLower(filepath.Ext(path))]
}

func sourceURI(base, path string) string {
	relativePath, err := filepath.Rel(base, path)
	if err != nil {
		return ""
	}
	return "file:///" + filepath.ToSlash(relativePath)
}

func sourcePrefix(base, target string) (string, error) {
	relativePath, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	if relativePath == "." {
		return "file:///", nil
	}
	return "file:///" + strings.TrimSuffix(filepath.ToSlash(relativePath), "/") + "/", nil
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("cryptographic random source unavailable")
	}
	return hex.EncodeToString(value[:])
}

var ignoredDirectories = map[string]bool{
	".git":         true,
	".cache":       true,
	".idea":        true,
	".vscode":      true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
}

var supportedExtensions = map[string]bool{
	".md":    true,
	".txt":   true,
	".go":    true,
	".js":    true,
	".jsx":   true,
	".ts":    true,
	".tsx":   true,
	".json":  true,
	".yaml":  true,
	".yml":   true,
	".sql":   true,
	".proto": true,
	".py":    true,
	".java":  true,
	".kt":    true,
	".rs":    true,
	".cs":    true,
	".c":     true,
	".cpp":   true,
	".h":     true,
	".hpp":   true,
	".html":  true,
	".css":   true,
	".scss":  true,
}
