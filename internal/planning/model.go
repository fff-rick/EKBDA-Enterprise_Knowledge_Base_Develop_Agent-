package planning

import (
	"time"

	"ekbda/internal/knowledge"
)

const (
	StatusAwaitingClarification = "awaiting_clarification"
	StatusAwaitingRoleReview    = "awaiting_role_review"
	StatusAwaitingResolution    = "awaiting_resolution"
	StatusAwaitingApproval      = "awaiting_approval"
	StatusApproved              = "approved"
	StatusRejected              = "rejected"
)

const (
	RoleResearchAnalyst    = "product_research_analyst"
	RoleProductManager     = "product_manager"
	RoleBackendEngineer    = "backend_engineer"
	RoleFrontendEngineer   = "frontend_engineer"
	RoleOperationsEngineer = "operations_engineer"
)

var requiredReviewRoles = [...]string{
	RoleResearchAnalyst,
	RoleProductManager,
	RoleBackendEngineer,
	RoleFrontendEngineer,
	RoleOperationsEngineer,
}

func RequiredReviewRoles() []string {
	return append([]string(nil), requiredReviewRoles[:]...)
}

type CreateInput struct {
	Project            string   `json:"project"`
	Repository         string   `json:"repository"`
	Technology         string   `json:"technology"`
	Title              string   `json:"title"`
	Requirement        string   `json:"requirement"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Constraints        []string `json:"constraints"`
	OutOfScope         []string `json:"out_of_scope"`
}

type Question struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Reason   string `json:"reason"`
}

type ClarificationAnswer struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

type ClarificationInput struct {
	Revision int                   `json:"revision"`
	Answers  []ClarificationAnswer `json:"answers"`
}

type DecisionInput struct {
	Revision int    `json:"revision"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type Resolution struct {
	DecisionID string `json:"decision_id"`
	Resolution string `json:"resolution"`
}

type ResolutionInput struct {
	Revision    int          `json:"revision"`
	Resolutions []Resolution `json:"resolutions"`
}

type KnowledgeReference struct {
	ID          string              `json:"id"`
	DocumentID  string              `json:"document_id"`
	Version     int                 `json:"version"`
	ChunkIndex  int                 `json:"chunk_index"`
	SnippetHash string              `json:"snippet_hash"`
	Title       string              `json:"title,omitempty"`
	Snippet     string              `json:"snippet,omitempty"`
	Citation    *knowledge.Citation `json:"citation,omitempty"`
}

type RuleReference struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Level       string `json:"level"`
}

type StandardReference struct {
	ID             string          `json:"id"`
	PackageID      string          `json:"package_id"`
	Name           string          `json:"name"`
	Scope          string          `json:"scope"`
	Selector       string          `json:"selector"`
	Version        int             `json:"version"`
	DefinitionHash string          `json:"definition_hash"`
	Rules          []RuleReference `json:"rules"`
}

type RepositoryContext struct {
	Repository       string   `json:"repository"`
	HeadCommit       string   `json:"head_commit,omitempty"`
	Branch           string   `json:"branch,omitempty"`
	Dirty            bool     `json:"dirty"`
	TrackedCount     int      `json:"tracked_count"`
	UntrackedCount   int      `json:"untracked_count"`
	ChangedCount     int      `json:"changed_count"`
	ChangedPaths     []string `json:"changed_paths"`
	ChangesTruncated bool     `json:"changes_truncated"`
	LastSyncID       string   `json:"last_sync_id,omitempty"`
	LastSyncCommit   string   `json:"last_sync_commit,omitempty"`
}

type ContextSnapshot struct {
	Hash       string               `json:"hash"`
	Knowledge  []KnowledgeReference `json:"knowledge"`
	Standards  []StandardReference  `json:"standards"`
	Repository RepositoryContext    `json:"repository"`
	CapturedAt time.Time            `json:"captured_at"`
}

type PlanStep struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	Deliverables        []string `json:"deliverables"`
	Verification        []string `json:"verification"`
	KnowledgeReferences []string `json:"knowledge_references"`
	StandardReferences  []string `json:"standard_references"`
}

type Risk struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Mitigation  string `json:"mitigation"`
}

type Plan struct {
	Summary     string     `json:"summary"`
	Assumptions []string   `json:"assumptions"`
	Steps       []PlanStep `json:"steps"`
	Risks       []Risk     `json:"risks"`
	OutOfScope  []string   `json:"out_of_scope"`
}

type ReviewFinding struct {
	ID             string `json:"id"`
	Severity       string `json:"severity"`
	Statement      string `json:"statement"`
	Recommendation string `json:"recommendation"`
}

type RoleReview struct {
	Role                string          `json:"role"`
	Summary             string          `json:"summary"`
	Recommendation      string          `json:"recommendation"`
	Findings            []ReviewFinding `json:"findings"`
	OpenQuestions       []string        `json:"open_questions"`
	KnowledgeReferences []string        `json:"knowledge_references"`
	StandardReferences  []string        `json:"standard_references"`
}

type DecisionItem struct {
	ID          string     `json:"id"`
	Topic       string     `json:"topic"`
	Description string     `json:"description"`
	Options     []string   `json:"options"`
	SourceRoles []string   `json:"source_roles"`
	Resolution  string     `json:"resolution,omitempty"`
	ResolvedBy  string     `json:"resolved_by,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

type Coordination struct {
	Summary       string         `json:"summary"`
	Consensus     []string       `json:"consensus"`
	DecisionItems []DecisionItem `json:"decision_items"`
}

type RoleReviewCycle struct {
	Provider     string          `json:"provider"`
	Context      ContextSnapshot `json:"context"`
	Reviews      []RoleReview    `json:"reviews"`
	Coordination Coordination    `json:"coordination"`
	CompletedAt  time.Time       `json:"completed_at"`
}

type Session struct {
	ID                 string                `json:"id"`
	Project            string                `json:"project"`
	Repository         string                `json:"repository"`
	Technology         string                `json:"technology"`
	Title              string                `json:"title"`
	Requirement        string                `json:"requirement"`
	AcceptanceCriteria []string              `json:"acceptance_criteria"`
	Constraints        []string              `json:"constraints"`
	OutOfScope         []string              `json:"out_of_scope"`
	Status             string                `json:"status"`
	Revision           int                   `json:"revision"`
	Questions          []Question            `json:"questions"`
	Answers            []ClarificationAnswer `json:"answers"`
	Plan               *Plan                 `json:"plan,omitempty"`
	RoleReview         *RoleReviewCycle      `json:"role_review,omitempty"`
	Context            ContextSnapshot       `json:"context"`
	Provider           string                `json:"provider"`
	CreatedBy          string                `json:"created_by"`
	ReviewedBy         string                `json:"reviewed_by,omitempty"`
	DecisionReason     string                `json:"decision_reason,omitempty"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
	ReviewedAt         *time.Time            `json:"reviewed_at,omitempty"`
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
