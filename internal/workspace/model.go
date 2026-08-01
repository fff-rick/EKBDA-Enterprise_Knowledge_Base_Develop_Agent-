package workspace

import (
	"time"

	"ekbda/internal/standards"
)

type ValidateInput struct {
	Repository string `json:"repository"`
	Project    string `json:"project"`
	Technology string `json:"technology"`
}

type Change struct {
	Path           string `json:"path"`
	IndexStatus    string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
}

type Snapshot struct {
	ID                string    `json:"id"`
	Repository        string    `json:"repository"`
	Project           string    `json:"project"`
	Technology        string    `json:"technology"`
	HeadCommit        string    `json:"head_commit,omitempty"`
	Branch            string    `json:"branch,omitempty"`
	Dirty             bool      `json:"dirty"`
	FileCount         int       `json:"file_count"`
	TrackedCount      int       `json:"tracked_count"`
	UntrackedCount    int       `json:"untracked_count"`
	BinaryCount       int       `json:"binary_count"`
	ChangedCount      int       `json:"changed_count"`
	Changes           []Change  `json:"changes"`
	InputHash         string    `json:"input_hash"`
	StandardsReportID string    `json:"standards_report_id"`
	Passed            bool      `json:"passed"`
	ValidatedBy       string    `json:"validated_by"`
	CreatedAt         time.Time `json:"created_at"`
}

type Result struct {
	Repository Snapshot                   `json:"repository"`
	Standards  standards.ValidationReport `json:"standards"`
}

// ContentSnapshot is a bounded, read-only view of the current Git worktree.
// Callers that require committed content must reject Dirty snapshots.
type ContentSnapshot struct {
	Repository     string
	HeadCommit     string
	Branch         string
	Dirty          bool
	TrackedCount   int
	UntrackedCount int
	BinaryCount    int
	Files          []standards.File
	Changes        []Change
}

type CommitChange struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type RepositoryState struct {
	Repository     string   `json:"repository"`
	HeadCommit     string   `json:"head_commit,omitempty"`
	Branch         string   `json:"branch,omitempty"`
	Dirty          bool     `json:"dirty"`
	TrackedCount   int      `json:"tracked_count"`
	UntrackedCount int      `json:"untracked_count"`
	Changes        []Change `json:"changes"`
}
