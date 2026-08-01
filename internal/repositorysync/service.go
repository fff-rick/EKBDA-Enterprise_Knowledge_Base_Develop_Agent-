package repositorysync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"ekbda/internal/knowledge"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

type Service struct {
	workspace *workspace.Service
	knowledge *knowledge.Service
	store     Store
	mu        sync.Mutex
	running   map[string]bool
}

func New(workspaceService *workspace.Service, knowledgeService *knowledge.Service, store Store) *Service {
	return &Service{workspace: workspaceService, knowledge: knowledgeService, store: store, running: make(map[string]bool)}
}

func (s *Service) Sync(ctx context.Context, input Input, syncedBy string) (Report, error) {
	input.Project = strings.ToLower(strings.TrimSpace(input.Project))
	input.Repository = strings.TrimSpace(strings.ReplaceAll(input.Repository, "\\", "/"))
	if input.Repository != "" {
		input.Repository = path.Clean(input.Repository)
	}
	input.BusinessDomain = strings.TrimSpace(input.BusinessDomain)
	input.AllowedRoles = normalizeRoles(input.AllowedRoles)
	if input.Project == "" || input.Repository == "" || input.Repository == ".." || strings.HasPrefix(input.Repository, "../") || strings.TrimSpace(syncedBy) == "" {
		return Report{}, ErrInvalidInput
	}
	key := input.Project + "\x00" + input.Repository
	if !s.begin(key) {
		return Report{}, ErrSyncInProgress
	}
	defer s.end(key)
	if input.Classification == "" {
		input.Classification = knowledge.ClassificationInternal
	}
	if input.Classification != knowledge.ClassificationPublic && input.Classification != knowledge.ClassificationInternal && input.Classification != knowledge.ClassificationRestricted {
		return Report{}, ErrInvalidInput
	}
	if input.Classification == knowledge.ClassificationRestricted && len(input.AllowedRoles) == 0 {
		return Report{}, ErrInvalidInput
	}

	startedAt := time.Now().UTC()
	content, err := s.workspace.Read(ctx, input.Repository)
	if err != nil {
		return Report{}, err
	}
	if content.Dirty || content.HeadCommit == "" {
		return Report{}, ErrDirtyRepository
	}
	report := Report{
		ID: newID(), Repository: content.Repository, Project: input.Project,
		BusinessDomain: input.BusinessDomain, Classification: input.Classification,
		AllowedRoles: append([]string(nil), input.AllowedRoles...),
		HeadCommit:   content.HeadCommit, Branch: content.Branch, FullResync: input.FullResync,
		Files: make([]FileResult, 0), SyncedBy: strings.TrimSpace(syncedBy), StartedAt: startedAt,
	}
	if !input.FullResync {
		previous, latestErr := s.store.LatestCompleted(ctx, input.Project, content.Repository)
		if latestErr == nil {
			report.PreviousHeadCommit = previous.HeadCommit
		} else if !errors.Is(latestErr, ErrReportNotFound) {
			return Report{}, latestErr
		}
	}
	if report.PreviousHeadCommit == "" {
		report.CommitChanges = initialChanges(content.Files)
	} else if report.PreviousHeadCommit != report.HeadCommit {
		report.CommitChanges, err = s.workspace.CommitDiff(ctx, content.Repository, report.PreviousHeadCommit, report.HeadCommit)
		if err != nil {
			return Report{}, fmt.Errorf("calculate commit-level repository changes: %w", err)
		}
	} else {
		report.CommitChanges = make([]workspace.CommitChange, 0)
	}

	seen := make(map[string]bool)
	prefix := sourcePrefix(content.Repository)
	for _, file := range content.Files {
		report.Scanned++
		result := s.importFile(ctx, input, content.Repository, file.Path, file.Content, seen)
		report.Files = append(report.Files, result)
		report.RedactionCount += result.RedactionCount
		if result.SkipReason == "sensitive_file" {
			report.SensitiveFilesSkipped++
		}
		switch result.Action {
		case knowledge.ImportActionCreated:
			report.Created++
		case knowledge.ImportActionUpdated:
			report.Updated++
		case knowledge.ImportActionSkipped:
			report.Skipped++
		default:
			if result.Error != "" {
				report.Failed++
			} else {
				report.Skipped++
			}
		}
	}
	deleted, err := s.knowledge.MarkMissingSources(ctx, input.Project, prefix, seen)
	if err != nil {
		return Report{}, fmt.Errorf("mark missing repository knowledge: %w", err)
	}
	for _, document := range deleted {
		report.Deleted++
		report.Files = append(report.Files, FileResult{
			Path: strings.TrimPrefix(document.SourceURI, prefix), Action: knowledge.ImportActionDeleted,
			DocumentID: document.ID, Version: document.Version,
		})
	}
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Path < report.Files[j].Path })
	report.Status = StatusCompleted
	if report.Failed > 0 {
		report.Status = StatusCompletedWithErrors
	}
	report.CompletedAt = time.Now().UTC()
	if err := s.store.Save(ctx, report); err != nil {
		return Report{}, fmt.Errorf("save repository knowledge sync report: %w", err)
	}
	return report, nil
}

func (s *Service) begin(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[key] {
		return false
	}
	s.running[key] = true
	return true
}

func (s *Service) end(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, key)
}

func (s *Service) Get(ctx context.Context, id string) (Report, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, project string, limit int) ([]Report, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.List(ctx, strings.ToLower(strings.TrimSpace(project)), limit)
}

func (s *Service) LatestCompleted(ctx context.Context, project, repository string) (Report, error) {
	return s.store.LatestCompleted(ctx, strings.ToLower(strings.TrimSpace(project)), strings.TrimSpace(strings.ReplaceAll(repository, "\\", "/")))
}

func (s *Service) importFile(ctx context.Context, input Input, repository, filePath, content string, seen map[string]bool) FileResult {
	if sensitiveFile(filePath) {
		return FileResult{Path: filePath, SkipReason: "sensitive_file"}
	}
	if !supportedFile(filePath) {
		return FileResult{Path: filePath, SkipReason: "unsupported_file"}
	}
	if strings.TrimSpace(content) == "" {
		return FileResult{Path: filePath, SkipReason: "non_text_or_empty"}
	}
	sourceURI := sourcePrefix(repository) + filePath
	seen[sourceURI] = true
	redacted, redactionCount := redact(content)
	digest, err := documentHash(redacted, input)
	if err != nil {
		return FileResult{Path: filePath, RedactionCount: redactionCount, Error: err.Error()}
	}
	document, action, err := s.knowledge.Import(ctx, knowledge.ImportDocumentInput{
		CreateDocumentInput: knowledge.CreateDocumentInput{
			Title: filePath, Content: redacted, SourceURI: sourceURI,
			BusinessDomain: input.BusinessDomain, Project: input.Project,
			Classification: input.Classification, AllowedRoles: input.AllowedRoles,
		},
		ContentHash: digest,
	})
	if err != nil {
		return FileResult{Path: filePath, RedactionCount: redactionCount, Error: err.Error()}
	}
	return FileResult{Path: filePath, Action: action, DocumentID: document.ID, Version: document.Version, RedactionCount: redactionCount}
}

func documentHash(content string, input Input) (string, error) {
	encoded, err := json.Marshal(struct {
		Content        string                   `json:"content"`
		BusinessDomain string                   `json:"business_domain"`
		Classification knowledge.Classification `json:"classification"`
		AllowedRoles   []string                 `json:"allowed_roles"`
	}{content, input.BusinessDomain, input.Classification, input.AllowedRoles})
	if err != nil {
		return "", fmt.Errorf("hash repository knowledge document: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func initialChanges(files []standards.File) []workspace.CommitChange {
	changes := make([]workspace.CommitChange, 0, len(files))
	for _, file := range files {
		changes = append(changes, workspace.CommitChange{Path: file.Path, Status: "A"})
	}
	return changes
}

func sourcePrefix(repository string) string { return "git://" + repository + "/" }

func supportedFile(filePath string) bool {
	base := strings.ToLower(path.Base(filePath))
	if base == "dockerfile" || base == "makefile" {
		return true
	}
	switch strings.ToLower(path.Ext(base)) {
	case ".md", ".mdx", ".txt", ".go", ".mod", ".sum", ".py", ".java", ".kt", ".kts", ".js", ".jsx", ".ts", ".tsx", ".vue", ".rs", ".cs", ".c", ".cc", ".cpp", ".h", ".hpp", ".html", ".css", ".scss", ".sql", ".yaml", ".yml", ".json", ".xml", ".toml", ".ini", ".conf", ".properties", ".proto", ".graphql", ".gradle", ".tf", ".hcl", ".sh", ".ps1":
		return true
	default:
		return false
	}
}

func normalizeRoles(roles []string) []string {
	seen := make(map[string]struct{}, len(roles))
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		if _, found := seen[role]; found {
			continue
		}
		seen[role] = struct{}{}
		result = append(result, role)
	}
	sort.Strings(result)
	return result
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("repository-sync-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
