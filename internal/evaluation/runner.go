package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ekbda/internal/answer"
)

var ErrInvalidSuite = errors.New("evaluation suite must contain 1 to 100 valid cases")

type Case struct {
	Name            string   `json:"name"`
	Query           string   `json:"query"`
	Project         string   `json:"project"`
	Roles           []string `json:"roles"`
	ExpectRefused   bool     `json:"expect_refused"`
	RequiredSources []string `json:"required_sources"`
	AnswerContains  []string `json:"answer_contains"`
}

type Request struct {
	Cases  []Case `json:"cases"`
	UserID string `json:"-"`
}

type CaseResult struct {
	Name            string   `json:"name"`
	Passed          bool     `json:"passed"`
	Failures        []string `json:"failures"`
	TraceID         string   `json:"trace_id,omitempty"`
	Refused         bool     `json:"refused"`
	CitationSources []string `json:"citation_sources"`
}

type Report struct {
	Total    int          `json:"total"`
	Passed   int          `json:"passed"`
	Failed   int          `json:"failed"`
	PassRate float64      `json:"pass_rate"`
	Results  []CaseResult `json:"results"`
}

type Runner struct {
	answers *answer.Service
}

func NewRunner(answers *answer.Service) *Runner {
	return &Runner{answers: answers}
}

func (r *Runner) Run(ctx context.Context, request Request) (Report, error) {
	if err := validate(request); err != nil {
		return Report{}, err
	}
	report := Report{Total: len(request.Cases), Results: make([]CaseResult, 0, len(request.Cases))}
	for _, testCase := range request.Cases {
		result := r.runCase(ctx, testCase, request.UserID)
		report.Results = append(report.Results, result)
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
	}
	report.PassRate = float64(report.Passed) / float64(report.Total)
	return report, nil
}

func (r *Runner) runCase(ctx context.Context, testCase Case, userID string) CaseResult {
	result := CaseResult{
		Name:            testCase.Name,
		Failures:        make([]string, 0),
		CitationSources: make([]string, 0),
	}
	if strings.TrimSpace(userID) == "" {
		userID = "evaluation"
	} else {
		userID = "evaluation:" + strings.TrimSpace(userID)
	}
	response, err := r.answers.Ask(ctx, answer.Input{
		Query:   testCase.Query,
		Project: testCase.Project,
		UserID:  userID,
		Roles:   testCase.Roles,
	})
	if err != nil {
		result.TraceID = answer.ErrorTraceID(err)
		result.Failures = append(result.Failures, "answer request failed")
		return result
	}
	result.TraceID = response.TraceID
	result.Refused = response.Refused
	for _, citation := range response.Citations {
		result.CitationSources = append(result.CitationSources, citation.Citation.SourceURI)
	}
	if response.Refused != testCase.ExpectRefused {
		result.Failures = append(result.Failures, fmt.Sprintf("expected refused=%t, got %t", testCase.ExpectRefused, response.Refused))
	}
	for _, required := range testCase.RequiredSources {
		if !containsExact(result.CitationSources, required) {
			result.Failures = append(result.Failures, "missing required source: "+required)
		}
	}
	answerText := strings.ToLower(response.Answer)
	for _, required := range testCase.AnswerContains {
		if !strings.Contains(answerText, strings.ToLower(required)) {
			result.Failures = append(result.Failures, "answer missing required text: "+required)
		}
	}
	result.Passed = len(result.Failures) == 0
	return result
}

func validate(request Request) error {
	if len(request.Cases) == 0 || len(request.Cases) > 100 {
		return ErrInvalidSuite
	}
	names := make(map[string]bool, len(request.Cases))
	for _, testCase := range request.Cases {
		name := strings.TrimSpace(testCase.Name)
		if name == "" || strings.TrimSpace(testCase.Query) == "" || strings.TrimSpace(testCase.Project) == "" || names[name] {
			return ErrInvalidSuite
		}
		names[name] = true
	}
	return nil
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
