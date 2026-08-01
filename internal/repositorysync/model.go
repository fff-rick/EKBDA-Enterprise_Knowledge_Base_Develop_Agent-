package repositorysync

import (
	"time"

	"ekbda/internal/knowledge"
	"ekbda/internal/workspace"
)

const (
	StatusCompleted           = "completed"
	StatusCompletedWithErrors = "completed_with_errors"
)

type Input struct {
	Repository     string                   `json:"repository"`
	Project        string                   `json:"project"`
	BusinessDomain string                   `json:"business_domain"`
	Classification knowledge.Classification `json:"classification"`
	AllowedRoles   []string                 `json:"allowed_roles"`
	FullResync     bool                     `json:"full_resync"`
}

type FileResult struct {
	Path           string                 `json:"path"`
	Action         knowledge.ImportAction `json:"action,omitempty"`
	DocumentID     string                 `json:"document_id,omitempty"`
	Version        int                    `json:"version,omitempty"`
	RedactionCount int                    `json:"redaction_count,omitempty"`
	SkipReason     string                 `json:"skip_reason,omitempty"`
	Error          string                 `json:"error,omitempty"`
}

type Report struct {
	ID                    string                   `json:"id"`
	Status                string                   `json:"status"`
	Repository            string                   `json:"repository"`
	Project               string                   `json:"project"`
	BusinessDomain        string                   `json:"business_domain"`
	Classification        knowledge.Classification `json:"classification"`
	AllowedRoles          []string                 `json:"allowed_roles,omitempty"`
	HeadCommit            string                   `json:"head_commit"`
	PreviousHeadCommit    string                   `json:"previous_head_commit,omitempty"`
	Branch                string                   `json:"branch,omitempty"`
	FullResync            bool                     `json:"full_resync"`
	CommitChanges         []workspace.CommitChange `json:"commit_changes"`
	Scanned               int                      `json:"scanned"`
	Created               int                      `json:"created"`
	Updated               int                      `json:"updated"`
	Skipped               int                      `json:"skipped"`
	Deleted               int                      `json:"deleted"`
	Failed                int                      `json:"failed"`
	SensitiveFilesSkipped int                      `json:"sensitive_files_skipped"`
	RedactionCount        int                      `json:"redaction_count"`
	Files                 []FileResult             `json:"files"`
	SyncedBy              string                   `json:"synced_by"`
	StartedAt             time.Time                `json:"started_at"`
	CompletedAt           time.Time                `json:"completed_at"`
}
