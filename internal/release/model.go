package release

import "time"

const (
	StatusAwaitingSourceVerification = "awaiting_source_verification"
	StatusAwaitingApproval           = "awaiting_approval"
	StatusApproved                   = "approved"
	StatusRejected                   = "rejected"
	StatusQueued                     = "queued"
	StatusRunning                    = "running"
	StatusSucceeded                  = "succeeded"
	StatusFailed                     = "failed"
	StatusRollbackAwaitingApproval   = "rollback_awaiting_approval"
	StatusRollbackApproved           = "rollback_approved"
	StatusRollbackQueued             = "rollback_queued"
	StatusRollbackRunning            = "rollback_running"
	StatusRolledBack                 = "rolled_back"
	StatusRollbackFailed             = "rollback_failed"
)

const (
	ProviderStatusQueued    = "queued"
	ProviderStatusRunning   = "running"
	ProviderStatusSucceeded = "succeeded"
	ProviderStatusFailed    = "failed"
	ProviderPhaseDeploy     = "deploy"
	ProviderPhaseRollback   = "rollback"
)

var RequiredChecks = []string{
	"configuration", "secret_scan", "image_scan", "migration",
	"monitoring", "rollback", "health", "smoke", "logs",
}

type CreateInput struct {
	DevelopmentSessionID string `json:"development_session_id"`
	Environment          string `json:"environment"`
	Pipeline             string `json:"pipeline"`
	ChangeTicket         string `json:"change_ticket"`
	ManifestSHA256       string `json:"manifest_sha256"`
	ConfigurationSHA256  string `json:"configuration_sha256"`
	RollbackPlan         string `json:"rollback_plan"`
	PromoteFromReleaseID string `json:"promote_from_release_id,omitempty"`
}

type DecisionInput struct {
	Revision int    `json:"revision"`
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

type TriggerInput struct {
	Revision     int    `json:"revision"`
	Confirmation string `json:"confirmation"`
}

type RollbackInput struct {
	Revision int    `json:"revision"`
	Reason   string `json:"reason"`
}

type ArtifactEvidence struct {
	URI                string `json:"uri"`
	Digest             string `json:"digest"`
	SBOMURI            string `json:"sbom_uri"`
	SBOMSHA256         string `json:"sbom_sha256"`
	SignatureVerified  bool   `json:"signature_verified"`
	ProvenanceVerified bool   `json:"provenance_verified"`
}

type CheckEvidence struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	EvidenceURI string `json:"evidence_uri,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type ProviderEvent struct {
	EventID   string            `json:"event_id"`
	ReleaseID string            `json:"release_id"`
	RunID     string            `json:"run_id"`
	Phase     string            `json:"phase"`
	Status    string            `json:"status"`
	Artifact  *ArtifactEvidence `json:"artifact,omitempty"`
	Checks    []CheckEvidence   `json:"checks,omitempty"`
	Message   string            `json:"message,omitempty"`
}

type CodePlatformEvent struct {
	EventID           string `json:"event_id"`
	ReleaseID         string `json:"release_id"`
	PullRequestURL    string `json:"pull_request_url"`
	HeadCommit        string `json:"head_commit"`
	MergeCommit       string `json:"merge_commit"`
	ProtectedBranch   bool   `json:"protected_branch"`
	Approvals         int    `json:"approvals"`
	RequiredApprovals int    `json:"required_approvals"`
	ChecksPassed      bool   `json:"checks_passed"`
	Merged            bool   `json:"merged"`
}

type SourceEvidence struct {
	EventID           string    `json:"event_id"`
	HeadCommit        string    `json:"head_commit"`
	MergeCommit       string    `json:"merge_commit"`
	ProtectedBranch   bool      `json:"protected_branch"`
	Approvals         int       `json:"approvals"`
	RequiredApprovals int       `json:"required_approvals"`
	ChecksPassed      bool      `json:"checks_passed"`
	VerifiedAt        time.Time `json:"verified_at"`
}

type Request struct {
	ID                     string            `json:"id"`
	DevelopmentSessionID   string            `json:"development_session_id"`
	Project                string            `json:"project"`
	Repository             string            `json:"repository"`
	SourceCommit           string            `json:"source_commit"`
	Commit                 string            `json:"commit"`
	PullRequestURL         string            `json:"pull_request_url"`
	Environment            string            `json:"environment"`
	Pipeline               string            `json:"pipeline"`
	ChangeTicket           string            `json:"change_ticket"`
	ManifestSHA256         string            `json:"manifest_sha256"`
	ConfigurationSHA256    string            `json:"configuration_sha256"`
	RollbackPlan           string            `json:"rollback_plan"`
	PromotedFromReleaseID  string            `json:"promoted_from_release_id,omitempty"`
	PromotedArtifactDigest string            `json:"promoted_artifact_digest,omitempty"`
	RequiredChecks         []string          `json:"required_checks"`
	Status                 string            `json:"status"`
	Revision               int               `json:"revision"`
	TriggerConfirmation    string            `json:"trigger_confirmation"`
	Source                 *SourceEvidence   `json:"source,omitempty"`
	CreatedBy              string            `json:"created_by"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
	ApprovedBy             string            `json:"approved_by,omitempty"`
	ApprovedAt             *time.Time        `json:"approved_at,omitempty"`
	ApprovalComment        string            `json:"approval_comment,omitempty"`
	RunID                  string            `json:"run_id,omitempty"`
	RunURL                 string            `json:"run_url,omitempty"`
	Artifact               *ArtifactEvidence `json:"artifact,omitempty"`
	Checks                 []CheckEvidence   `json:"checks,omitempty"`
	ErrorCode              string            `json:"error_code,omitempty"`
	ErrorMessage           string            `json:"error_message,omitempty"`
	RollbackRequestedBy    string            `json:"rollback_requested_by,omitempty"`
	RollbackReason         string            `json:"rollback_reason,omitempty"`
	RollbackApprovedBy     string            `json:"rollback_approved_by,omitempty"`
	RollbackRunID          string            `json:"rollback_run_id,omitempty"`
	RollbackRunURL         string            `json:"rollback_run_url,omitempty"`
	CompletedAt            *time.Time        `json:"completed_at,omitempty"`
}

type Event struct {
	ID              string    `json:"id"`
	ReleaseID       string    `json:"release_id"`
	Sequence        int       `json:"sequence"`
	Type            string    `json:"type"`
	FromStatus      string    `json:"from_status,omitempty"`
	ToStatus        string    `json:"to_status"`
	Actor           string    `json:"actor"`
	Reason          string    `json:"reason,omitempty"`
	ProviderEventID string    `json:"provider_event_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type ProviderRun struct {
	ID  string `json:"run_id"`
	URL string `json:"run_url"`
}

type TriggerRequest struct {
	ReleaseID                string   `json:"release_id"`
	Project                  string   `json:"project"`
	Repository               string   `json:"repository"`
	Commit                   string   `json:"commit"`
	PullRequestURL           string   `json:"pull_request_url"`
	Environment              string   `json:"environment"`
	Pipeline                 string   `json:"pipeline"`
	ChangeTicket             string   `json:"change_ticket"`
	ManifestSHA256           string   `json:"manifest_sha256"`
	ConfigurationSHA256      string   `json:"configuration_sha256"`
	RequiredChecks           []string `json:"required_checks"`
	ExpectedArtifactDigest   string   `json:"expected_artifact_digest,omitempty"`
	RequireArtifactSignature bool     `json:"require_artifact_signature"`
	RequireProvenance        bool     `json:"require_provenance"`
	RequireSBOM              bool     `json:"require_sbom"`
}

type RollbackRequest struct {
	ReleaseID      string `json:"release_id"`
	Project        string `json:"project"`
	Environment    string `json:"environment"`
	Pipeline       string `json:"pipeline"`
	RunID          string `json:"run_id"`
	ArtifactDigest string `json:"artifact_digest"`
	Reason         string `json:"reason"`
}
