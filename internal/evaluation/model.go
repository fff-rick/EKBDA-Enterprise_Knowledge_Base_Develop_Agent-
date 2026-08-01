package evaluation

import "time"

const (
	RunPending   = "pending"
	RunRunning   = "running"
	RunCompleted = "completed"
	RunFailed    = "failed"
	RunCanceled  = "canceled"

	GatePending  = "pending"
	GatePassed   = "passed"
	GateFailed   = "failed"
	GateError    = "error"
	GateCanceled = "canceled"
)

type CreateSuiteInput struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	MinimumPassRate *float64 `json:"minimum_pass_rate"`
	Cases           []Case   `json:"cases"`
}

type Suite struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Version         int       `json:"version"`
	DefinitionHash  string    `json:"definition_hash"`
	MinimumPassRate float64   `json:"minimum_pass_rate"`
	Cases           []Case    `json:"cases"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}

type SuiteSummary struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Version         int       `json:"version"`
	DefinitionHash  string    `json:"definition_hash"`
	MinimumPassRate float64   `json:"minimum_pass_rate"`
	CaseCount       int       `json:"case_count"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}

type StartRunInput struct {
	SuiteID string `json:"suite_id"`
}

type Run struct {
	ID              string    `json:"id"`
	SuiteID         string    `json:"suite_id"`
	SuiteName       string    `json:"suite_name"`
	SuiteVersion    int       `json:"suite_version"`
	DefinitionHash  string    `json:"definition_hash"`
	MinimumPassRate float64   `json:"minimum_pass_rate"`
	Status          string    `json:"status"`
	GateStatus      string    `json:"gate_status"`
	ErrorCode       string    `json:"error_code,omitempty"`
	RetryOfRunID    string    `json:"retry_of_run_id,omitempty"`
	Attempt         int       `json:"attempt"`
	CancelRequested bool      `json:"cancel_requested"`
	WorkerID        string    `json:"worker_id,omitempty"`
	LeaseUntil      time.Time `json:"lease_until,omitempty"`
	TriggeredBy     string    `json:"triggered_by"`
	Report          Report    `json:"report"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
}

type RunSummary struct {
	ID              string    `json:"id"`
	SuiteID         string    `json:"suite_id"`
	SuiteName       string    `json:"suite_name"`
	SuiteVersion    int       `json:"suite_version"`
	DefinitionHash  string    `json:"definition_hash"`
	MinimumPassRate float64   `json:"minimum_pass_rate"`
	Status          string    `json:"status"`
	GateStatus      string    `json:"gate_status"`
	ErrorCode       string    `json:"error_code,omitempty"`
	RetryOfRunID    string    `json:"retry_of_run_id,omitempty"`
	Attempt         int       `json:"attempt"`
	CancelRequested bool      `json:"cancel_requested"`
	TriggeredBy     string    `json:"triggered_by"`
	Total           int       `json:"total"`
	Passed          int       `json:"passed"`
	Failed          int       `json:"failed"`
	PassRate        float64   `json:"pass_rate"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
}

func (suite Suite) Summary() SuiteSummary {
	return SuiteSummary{
		ID: suite.ID, Name: suite.Name, Description: suite.Description,
		Version: suite.Version, DefinitionHash: suite.DefinitionHash,
		MinimumPassRate: suite.MinimumPassRate, CaseCount: len(suite.Cases),
		CreatedBy: suite.CreatedBy, CreatedAt: suite.CreatedAt,
	}
}

func (run Run) Summary() RunSummary {
	return RunSummary{
		ID: run.ID, SuiteID: run.SuiteID, SuiteName: run.SuiteName,
		SuiteVersion: run.SuiteVersion, DefinitionHash: run.DefinitionHash,
		MinimumPassRate: run.MinimumPassRate, Status: run.Status,
		GateStatus: run.GateStatus, ErrorCode: run.ErrorCode,
		RetryOfRunID: run.RetryOfRunID, Attempt: run.Attempt,
		CancelRequested: run.CancelRequested,
		TriggeredBy:     run.TriggeredBy, Total: run.Report.Total,
		Passed: run.Report.Passed, Failed: run.Report.Failed,
		PassRate: run.Report.PassRate, CreatedAt: run.CreatedAt,
		StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
	}
}
