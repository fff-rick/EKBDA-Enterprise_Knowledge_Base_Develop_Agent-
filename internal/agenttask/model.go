package agenttask

import (
	"encoding/json"
	"time"
)

const (
	KindRoleReview     = "role_review"
	KindProjectPackage = "project_package"

	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

type QualityCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}

type QualityReport struct {
	Passed bool           `json:"passed"`
	Checks []QualityCheck `json:"checks"`
}

type Task struct {
	ID               string          `json:"id"`
	Kind             string          `json:"kind"`
	Step             string          `json:"step"`
	Project          string          `json:"project"`
	Repository       string          `json:"repository"`
	Status           string          `json:"status"`
	ErrorCode        string          `json:"error_code,omitempty"`
	Input            json.RawMessage `json:"-"`
	ResourceID       string          `json:"resource_id,omitempty"`
	RetryOfTaskID    string          `json:"retry_of_task_id,omitempty"`
	Attempt          int             `json:"attempt"`
	CancelRequested  bool            `json:"cancel_requested"`
	WorkerID         string          `json:"worker_id,omitempty"`
	LeaseUntil       time.Time       `json:"lease_until,omitempty"`
	TriggeredBy      string          `json:"triggered_by"`
	RetryRequestedBy string          `json:"retry_requested_by,omitempty"`
	Usage            Usage           `json:"usage"`
	Quality          QualityReport   `json:"quality"`
	CreatedAt        time.Time       `json:"created_at"`
	StartedAt        time.Time       `json:"started_at,omitempty"`
	CompletedAt      time.Time       `json:"completed_at,omitempty"`
}

type ExecutionResult struct {
	ResourceID string
	Quality    QualityReport
}

type RoleReviewInput struct {
	SessionID          string   `json:"session_id"`
	Revision           int      `json:"revision"`
	Roles              []string `json:"roles"`
	GovernanceOverride bool     `json:"governance_override"`
}

type ProjectPackageInput struct {
	SessionID     string `json:"session_id"`
	Name          string `json:"name"`
	ChangeSummary string `json:"change_summary"`
}

type Pricing struct {
	InputUSDPerMillionTokens  float64
	OutputUSDPerMillionTokens float64
}
