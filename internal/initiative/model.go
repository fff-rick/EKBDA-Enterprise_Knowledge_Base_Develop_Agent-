package initiative

import "time"

const (
	ArtifactPRD          = "prd"
	ArtifactArchitecture = "architecture"
	ArtifactAPI          = "api"
	ArtifactTest         = "test"
	ArtifactDeployment   = "deployment"
	ArtifactMonitoring   = "monitoring"
	ArtifactRisk         = "risk"
)

var requiredArtifactTypes = [...]string{
	ArtifactPRD,
	ArtifactArchitecture,
	ArtifactAPI,
	ArtifactTest,
	ArtifactDeployment,
	ArtifactMonitoring,
	ArtifactRisk,
}

func RequiredArtifactTypes() []string {
	return append([]string(nil), requiredArtifactTypes[:]...)
}

const (
	ReferencePlanKnowledge   = "plan_knowledge"
	ReferencePlanStandard    = "plan_standard"
	ReferenceReviewKnowledge = "review_knowledge"
	ReferenceReviewStandard  = "review_standard"
	ReferenceDecision        = "decision"
)

type CreateInput struct {
	SessionID     string `json:"session_id"`
	Name          string `json:"name"`
	ChangeSummary string `json:"change_summary"`
}

type Reference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Section struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

type Artifact struct {
	Type       string      `json:"type"`
	Title      string      `json:"title"`
	Summary    string      `json:"summary"`
	Sections   []Section   `json:"sections"`
	References []Reference `json:"references"`
}

type TraceRecord struct {
	RequirementID          string   `json:"requirement_id"`
	Requirement            string   `json:"requirement"`
	ArchitectureSections   []string `json:"architecture_sections"`
	APIApplicable          bool     `json:"api_applicable"`
	APISections            []string `json:"api_sections"`
	APINotApplicableReason string   `json:"api_not_applicable_reason,omitempty"`
	TestSections           []string `json:"test_sections"`
	DeploymentSections     []string `json:"deployment_sections"`
	CoverageStatus         string   `json:"coverage_status"`
	Gaps                   []string `json:"gaps"`
}

type SourceSnapshot struct {
	PlanningSessionID string     `json:"planning_session_id"`
	PlanningRevision  int        `json:"planning_revision"`
	PlanContextHash   string     `json:"plan_context_hash"`
	ReviewContextHash string     `json:"review_context_hash"`
	PlanApprovedBy    string     `json:"plan_approved_by"`
	PlanApprovedAt    *time.Time `json:"plan_approved_at"`
}

type Package struct {
	ID             string         `json:"id"`
	Project        string         `json:"project"`
	Repository     string         `json:"repository"`
	Name           string         `json:"name"`
	Version        int            `json:"version"`
	ChangeSummary  string         `json:"change_summary"`
	DefinitionHash string         `json:"definition_hash"`
	Provider       string         `json:"provider"`
	Source         SourceSnapshot `json:"source"`
	Artifacts      []Artifact     `json:"artifacts"`
	Traceability   []TraceRecord  `json:"traceability"`
	CreatedBy      string         `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
}

type ReviewInput struct {
	ArtifactType string `json:"artifact_type"`
	PackageHash  string `json:"package_hash"`
	Decision     string `json:"decision"`
	Comment      string `json:"comment"`
}

type ArtifactReview struct {
	ID           string    `json:"id"`
	PackageID    string    `json:"package_id"`
	ArtifactType string    `json:"artifact_type"`
	PackageHash  string    `json:"package_hash"`
	Sequence     int       `json:"sequence"`
	Decision     string    `json:"decision"`
	Comment      string    `json:"comment"`
	ReviewedBy   string    `json:"reviewed_by"`
	ReviewedAt   time.Time `json:"reviewed_at"`
}

type ExportedDocument struct {
	Filename    string
	ContentType string
	Data        []byte
}
