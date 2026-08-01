package development

import "time"

const (
	StatusDraft            = "draft"
	StatusAwaitingApproval = "awaiting_approval"
	StatusApproved         = "approved"
	StatusRejected         = "rejected"
	StatusExecuting        = "executing"
	StatusVerified         = "verified"
	StatusExecutionFailed  = "execution_failed"
	StatusDelivering       = "delivering"
	StatusDelivered        = "delivered"
	StatusDeliveryFailed   = "delivery_failed"
)

const (
	ExecutionRunning = "running"
	ExecutionPassed  = "passed"
	ExecutionFailed  = "failed"
	DeliveryRunning  = "running"
	DeliveryPassed   = "passed"
	DeliveryFailed   = "failed"
)

const (
	EventCreated            = "created"
	EventProposalSubmitted  = "proposal_submitted"
	EventApproved           = "approved"
	EventRejected           = "rejected"
	EventExecutionStarted   = "execution_started"
	EventExecutionPassed    = "execution_passed"
	EventExecutionFailed    = "execution_failed"
	EventExecutionRecovered = "execution_recovered"
	EventDeliveryStarted    = "delivery_started"
	EventDeliveryPassed     = "delivery_passed"
	EventDeliveryFailed     = "delivery_failed"
	EventDeliveryRecovered  = "delivery_recovered"
)

type CreateInput struct {
	ProjectPackageID string   `json:"project_package_id"`
	Technology       string   `json:"technology"`
	AllowedPaths     []string `json:"allowed_paths"`
	AllowedCommands  []string `json:"allowed_commands"`
}

type SubmitInput struct {
	Revision   int      `json:"revision"`
	Summary    string   `json:"summary"`
	Patch      string   `json:"patch"`
	CommandIDs []string `json:"command_ids"`
}

type DecisionInput struct {
	Revision int    `json:"revision"`
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

type ExecuteInput struct {
	Revision     int    `json:"revision"`
	PatchHash    string `json:"patch_hash"`
	Confirmation string `json:"confirmation"`
}

type DeliverInput struct {
	Revision     int    `json:"revision"`
	PatchHash    string `json:"patch_hash"`
	Confirmation string `json:"confirmation"`
}

type FileChange struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type Command struct {
	ID         string   `json:"id"`
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments"`
	Purpose    string   `json:"purpose"`
}

type CommandEvidence struct {
	ID           string `json:"id"`
	ExitCode     int    `json:"exit_code"`
	TimedOut     bool   `json:"timed_out"`
	DurationMS   int64  `json:"duration_ms"`
	StdoutBytes  int    `json:"stdout_bytes"`
	StderrBytes  int    `json:"stderr_bytes"`
	StdoutSHA256 string `json:"stdout_sha256"`
	StderrSHA256 string `json:"stderr_sha256"`
}

type SecretScanEvidence struct {
	Scanner      string `json:"scanner"`
	Passed       bool   `json:"passed"`
	DurationMS   int64  `json:"duration_ms"`
	OutputBytes  int    `json:"output_bytes"`
	OutputSHA256 string `json:"output_sha256"`
}

type Execution struct {
	ID                  string              `json:"id"`
	Status              string              `json:"status"`
	BaselineCommit      string              `json:"baseline_commit"`
	PatchHash           string              `json:"patch_hash"`
	Isolation           string              `json:"isolation"`
	NetworkPolicy       string              `json:"network_policy"`
	SecretScanPassed    bool                `json:"secret_scan_passed"`
	SecretScan          *SecretScanEvidence `json:"secret_scan,omitempty"`
	StandardsReportID   string              `json:"standards_report_id,omitempty"`
	StandardsPassed     bool                `json:"standards_passed"`
	Commands            []CommandEvidence   `json:"commands"`
	ErrorCode           string              `json:"error_code,omitempty"`
	ErrorMessage        string              `json:"error_message,omitempty"`
	ExecutedBy          string              `json:"executed_by"`
	StartedAt           time.Time           `json:"started_at"`
	FinishedAt          time.Time           `json:"finished_at"`
	DurationMS          int64               `json:"duration_ms"`
	IsolatedCopyRemoved bool                `json:"isolated_copy_removed"`
}

type Delivery struct {
	ID                 string              `json:"id"`
	Status             string              `json:"status"`
	Branch             string              `json:"branch"`
	Commit             string              `json:"commit,omitempty"`
	Remote             string              `json:"remote"`
	BranchPushed       bool                `json:"branch_pushed"`
	PullRequestURL     string              `json:"pull_request_url,omitempty"`
	SecretScan         *SecretScanEvidence `json:"secret_scan,omitempty"`
	ErrorCode          string              `json:"error_code,omitempty"`
	ErrorMessage       string              `json:"error_message,omitempty"`
	DeliveredBy        string              `json:"delivered_by"`
	StartedAt          time.Time           `json:"started_at"`
	FinishedAt         time.Time           `json:"finished_at"`
	DurationMS         int64               `json:"duration_ms"`
	WorkingCopyRemoved bool                `json:"working_copy_removed"`
}

type Proposal struct {
	Summary     string       `json:"summary"`
	PatchHash   string       `json:"patch_hash"`
	PatchBytes  int          `json:"patch_bytes"`
	Files       []FileChange `json:"files"`
	Commands    []Command    `json:"commands"`
	SubmittedBy string       `json:"submitted_by"`
	SubmittedAt time.Time    `json:"submitted_at"`
	Patch       string       `json:"-"`
}

type Session struct {
	ID                 string     `json:"id"`
	Project            string     `json:"project"`
	Repository         string     `json:"repository"`
	ProjectPackageID   string     `json:"project_package_id"`
	ProjectPackageName string     `json:"project_package_name"`
	Technology         string     `json:"technology"`
	PackageVersion     int        `json:"package_version"`
	PackageHash        string     `json:"package_hash"`
	PlanningSessionID  string     `json:"planning_session_id"`
	BaselineCommit     string     `json:"baseline_commit"`
	BaselineBranch     string     `json:"baseline_branch,omitempty"`
	PlannedBranch      string     `json:"planned_branch"`
	AllowedPaths       []string   `json:"allowed_paths"`
	AllowedCommands    []string   `json:"allowed_commands"`
	Status             string     `json:"status"`
	Revision           int        `json:"revision"`
	Proposal           *Proposal  `json:"proposal,omitempty"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ReviewedBy         string     `json:"reviewed_by,omitempty"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	ReviewComment      string     `json:"review_comment,omitempty"`
	Execution          *Execution `json:"execution,omitempty"`
	Delivery           *Delivery  `json:"delivery,omitempty"`
}

type Event struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Sequence   int       `json:"sequence"`
	Type       string    `json:"type"`
	FromStatus string    `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status"`
	Actor      string    `json:"actor"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Preview struct {
	SessionID      string       `json:"session_id"`
	Revision       int          `json:"revision"`
	BaselineCommit string       `json:"baseline_commit"`
	PatchHash      string       `json:"patch_hash"`
	Patch          string       `json:"patch"`
	Files          []FileChange `json:"files"`
	Commands       []Command    `json:"commands"`
}
