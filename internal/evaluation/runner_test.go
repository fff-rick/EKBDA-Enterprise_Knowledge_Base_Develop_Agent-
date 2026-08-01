package evaluation

import (
	"context"
	"errors"
	"testing"

	"ekbda/internal/answer"
	"ekbda/internal/embedding"
	"ekbda/internal/knowledge"
)

func TestRunnerChecksCitationAndRefusal(t *testing.T) {
	knowledgeService := knowledge.NewService(knowledge.NewMemoryRepository(), embedding.NewLocalHash())
	_, err := knowledgeService.Create(context.Background(), knowledge.CreateDocumentInput{
		Title:     "Order startup",
		Content:   "Run go run ./cmd/server to start the order service.",
		SourceURI: "git://order/README.md",
		Project:   "order",
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	answerService := answer.NewService(knowledgeService, answer.NewLocalExtractive(), answer.NewMemoryTraceStore())
	runner := NewRunner(answerService)
	report, err := runner.Run(context.Background(), Request{Cases: []Case{
		{
			Name:            "grounded startup answer",
			Query:           "How do I start the order service?",
			Project:         "order",
			RequiredSources: []string{"git://order/README.md"},
			AnswerContains:  []string{"go run"},
		},
		{
			Name:          "unknown secret is refused",
			Query:         "What is the production password?",
			Project:       "missing-project",
			ExpectRefused: true,
		},
	}})
	if err != nil {
		t.Fatalf("run evaluation: %v", err)
	}
	if report.Total != 2 || report.Passed != 2 || report.PassRate != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for _, result := range report.Results {
		if result.TraceID == "" {
			t.Fatalf("evaluation result is missing trace: %#v", result)
		}
	}
}

func TestRunnerRejectsDuplicateCaseNames(t *testing.T) {
	runner := NewRunner(nil)
	_, err := runner.Run(context.Background(), Request{Cases: []Case{
		{Name: "duplicate", Query: "a", Project: "p"},
		{Name: "duplicate", Query: "b", Project: "p"},
	}})
	if !errors.Is(err, ErrInvalidSuite) {
		t.Fatalf("expected invalid suite, got %v", err)
	}
}
