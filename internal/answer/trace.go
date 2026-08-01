package answer

import (
	"context"
	"errors"
	"time"
)

var ErrTraceNotFound = errors.New("answer trace not found")

type TraceError struct {
	TraceID string
	Err     error
}

func (e *TraceError) Error() string {
	return e.Err.Error()
}

func (e *TraceError) Unwrap() error {
	return e.Err
}

func ErrorTraceID(err error) string {
	var traced *TraceError
	if errors.As(err, &traced) {
		return traced.TraceID
	}
	return ""
}

type Trace struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	QueryHash         string    `json:"query_hash"`
	QueryLength       int       `json:"query_length"`
	Project           string    `json:"project"`
	Provider          string    `json:"provider"`
	Status            string    `json:"status"`
	ErrorCode         string    `json:"error_code,omitempty"`
	Refused           bool      `json:"refused"`
	RefusalReason     string    `json:"refusal_reason,omitempty"`
	EvidenceCount     int       `json:"evidence_count"`
	CitationCount     int       `json:"citation_count"`
	PromptTokens      int       `json:"prompt_tokens"`
	CompletionTokens  int       `json:"completion_tokens"`
	TotalTokens       int       `json:"total_tokens"`
	InputRateUSD      float64   `json:"input_usd_per_million_tokens"`
	OutputRateUSD     float64   `json:"output_usd_per_million_tokens"`
	PromptCostUSD     float64   `json:"prompt_cost_usd"`
	CompletionCostUSD float64   `json:"completion_cost_usd"`
	TotalCostUSD      float64   `json:"total_cost_usd"`
	DurationMS        int64     `json:"duration_ms"`
	CreatedAt         time.Time `json:"created_at"`
}

type Metrics struct {
	Project         string         `json:"project,omitempty"`
	Total           int            `json:"total"`
	Succeeded       int            `json:"succeeded"`
	Errors          int            `json:"errors"`
	Refused         int            `json:"refused"`
	AverageDuration float64        `json:"average_duration_ms"`
	TotalTokens     int64          `json:"total_tokens"`
	TotalCostUSD    float64        `json:"total_cost_usd"`
	ByProvider      map[string]int `json:"by_provider"`
}

type Pricing struct {
	InputUSDPerMillionTokens  float64
	OutputUSDPerMillionTokens float64
}

type TraceStore interface {
	Save(ctx context.Context, trace Trace) error
	Get(ctx context.Context, id string) (Trace, error)
	Metrics(ctx context.Context, project string) (Metrics, error)
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}
