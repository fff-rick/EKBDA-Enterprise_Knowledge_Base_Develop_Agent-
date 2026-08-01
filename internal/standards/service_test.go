package standards

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func executablePackage() CreatePackageInput {
	return CreatePackageInput{
		Name: "go-service", Scope: ScopeTechnology, Selector: "go", Owner: "platform-team",
		Rules: []Rule{
			{ID: "readme-required", Title: "README is required", Category: CategoryDirectory, Level: LevelBlock, Check: &RuleCheck{Type: CheckRequiredPath, Pattern: "README.md"}},
			{ID: "dotenv-forbidden", Title: "Do not commit dotenv", Category: CategoryWorkflow, Level: LevelCheck, Check: &RuleCheck{Type: CheckForbiddenPath, Pattern: `(^|/)\.env$`}},
			{ID: "go-file-naming", Title: "Go files use snake case", Category: CategoryNaming, Level: LevelBlock, Check: &RuleCheck{Type: CheckPathPattern, Target: `\.go$`, Pattern: `(^|/)[a-z0-9_]+\.go$`}},
			{ID: "testing-doc", Title: "Document testing", Category: CategoryComment, Level: LevelCheck, Check: &RuleCheck{Type: CheckContent, Target: `^README\.md$`, Pattern: `(?m)^## Testing$`}},
			{ID: "unit-test-required", Title: "At least one unit test", Category: CategoryTesting, Level: LevelBlock, Check: &RuleCheck{Type: CheckMinimumMatch, Target: `_test\.go$`, Minimum: 1}},
			{ID: "review-guidance", Title: "Use two reviewers", Category: CategoryWorkflow, Level: LevelGuidance},
		},
	}
}

func TestCreatePackageVersionsImmutableDefinitions(t *testing.T) {
	service := NewService(NewMemoryStore())
	input := executablePackage()
	first, err := service.CreatePackage(context.Background(), input, "admin-1")
	if err != nil {
		t.Fatalf("create first package: %v", err)
	}
	input.Rules[0].Title = "Changed title"
	second, err := service.CreatePackage(context.Background(), input, "admin-1")
	if err != nil {
		t.Fatalf("create second package: %v", err)
	}
	if first.Version != 1 || second.Version != 2 || first.DefinitionHash == second.DefinitionHash {
		t.Fatalf("unexpected package versions: first=%#v second=%#v", first, second)
	}
	stored, err := service.GetPackage(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("get first package: %v", err)
	}
	if stored.Rules[0].Title != "README is required" {
		t.Fatalf("published package was mutated: %#v", stored)
	}
	if stored.Rules[0].Owner != "platform-team" {
		t.Fatalf("rule did not inherit package owner: %#v", stored.Rules[0])
	}
}

func TestValidateAppliesLatestPackageAndPersistsReport(t *testing.T) {
	service := NewService(NewMemoryStore())
	input := executablePackage()
	if _, err := service.CreatePackage(context.Background(), input, "admin-1"); err != nil {
		t.Fatalf("create package: %v", err)
	}
	report, err := service.Validate(context.Background(), ValidateInput{
		Project: "Order-Service", Technology: "Go",
		Files: []File{
			{Path: "README.md", Content: "# Service"},
			{Path: "internal/BadName.go", Content: "package internal"},
			{Path: ".env", Content: "SECRET=redacted"},
		},
	}, "developer-1")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if report.Passed || report.RuleCount != 6 || report.ViolationCount != 4 || report.BlockingCount != 2 || len(report.Packages) != 1 || report.InputHash == "" {
		t.Fatalf("unexpected report: %#v", report)
	}
	stored, err := service.GetReport(context.Background(), report.ID)
	if err != nil || stored.ValidatedBy != "developer-1" || len(stored.Violations) != 4 {
		t.Fatalf("unexpected stored report: %#v err=%v", stored, err)
	}
}

func TestValidatePassesCompliantManifest(t *testing.T) {
	service := NewService(NewMemoryStore())
	if _, err := service.CreatePackage(context.Background(), executablePackage(), "admin-1"); err != nil {
		t.Fatalf("create package: %v", err)
	}
	report, err := service.Validate(context.Background(), ValidateInput{
		Project: "order-service", Technology: "go",
		Files: []File{
			{Path: "README.md", Content: "# Service\n\n## Testing"},
			{Path: "internal/order_service.go", Content: "package internal"},
			{Path: "internal/order_service_test.go", Content: "package internal"},
		},
	}, "developer-1")
	if err != nil || !report.Passed || report.ViolationCount != 0 {
		t.Fatalf("expected passing report: %#v err=%v", report, err)
	}
}

func TestValidateUsesLatestApplicablePackagesAcrossLayers(t *testing.T) {
	service := NewService(NewMemoryStore())
	common := CreatePackageInput{Name: "security", Scope: ScopeCommon, Owner: "security", Rules: []Rule{
		{ID: "no-secret", Title: "No secret files", Category: CategoryWorkflow, Level: LevelBlock, Check: &RuleCheck{Type: CheckForbiddenPath, Pattern: `secret`}},
	}}
	technologyV1 := CreatePackageInput{Name: "go-layout", Scope: ScopeTechnology, Selector: "go", Owner: "go-team", Rules: []Rule{
		{ID: "old-layout", Title: "Old layout", Category: CategoryDirectory, Level: LevelBlock, Check: &RuleCheck{Type: CheckRequiredPath, Pattern: "old.txt"}},
	}}
	technologyV2 := technologyV1
	technologyV2.Rules = cloneRules(technologyV1.Rules)
	technologyV2.Rules[0].ID = "new-layout"
	technologyV2.Rules[0].Check.Pattern = "go.mod"
	project := CreatePackageInput{Name: "order-policy", Scope: ScopeProject, Selector: "order", Owner: "order-team", Rules: []Rule{
		{ID: "runbook", Title: "Runbook required", Category: CategoryDirectory, Level: LevelBlock, Check: &RuleCheck{Type: CheckRequiredPath, Pattern: "RUNBOOK.md"}},
	}}
	for _, input := range []CreatePackageInput{common, technologyV1, technologyV2, project} {
		if _, err := service.CreatePackage(context.Background(), input, "admin-1"); err != nil {
			t.Fatalf("create layered package: %v", err)
		}
	}
	report, err := service.Validate(context.Background(), ValidateInput{
		Project: "order", Technology: "go", Files: []File{{Path: "go.mod"}, {Path: "RUNBOOK.md"}},
	}, "developer-1")
	if err != nil || !report.Passed || report.RuleCount != 3 || len(report.Packages) != 3 {
		t.Fatalf("unexpected layered report: %#v err=%v", report, err)
	}
	for _, reference := range report.Packages {
		if reference.Name == "go-layout" && reference.Version != 2 {
			t.Fatalf("validation used stale package: %#v", reference)
		}
	}
}

func TestValidateRejectsDuplicateApplicableRuleIDs(t *testing.T) {
	service := NewService(NewMemoryStore())
	for _, input := range []CreatePackageInput{
		{Name: "common", Scope: ScopeCommon, Owner: "team", Rules: []Rule{{ID: "same", Title: "Common", Category: CategoryDirectory, Level: LevelGuidance}}},
		{Name: "project", Scope: ScopeProject, Selector: "app", Owner: "team", Rules: []Rule{{ID: "same", Title: "Project", Category: CategoryDirectory, Level: LevelGuidance}}},
	} {
		if _, err := service.CreatePackage(context.Background(), input, "admin"); err != nil {
			t.Fatalf("create package: %v", err)
		}
	}
	_, err := service.Validate(context.Background(), ValidateInput{Project: "app", Technology: "go", Files: []File{{Path: "README.md"}}}, "developer")
	if !errors.Is(err, ErrApplicableRuleConflict) {
		t.Fatalf("expected rule conflict, got %v", err)
	}
}

func TestCreatePackageRejectsUnsafeDefinitions(t *testing.T) {
	service := NewService(NewMemoryStore())
	tests := []CreatePackageInput{
		{Name: "bad", Scope: ScopeTechnology, Selector: "", Owner: "team", Rules: []Rule{{ID: "rule", Title: "Rule", Category: CategoryDirectory, Level: LevelGuidance}}},
		{Name: "bad", Scope: ScopeCommon, Owner: "team", Rules: []Rule{{ID: "rule", Title: "Rule", Category: CategoryDirectory, Level: LevelBlock, Check: &RuleCheck{Type: CheckRequiredPath, Pattern: "../secret"}}}},
		{Name: "bad", Scope: ScopeCommon, Owner: "team", Rules: []Rule{{ID: "rule", Title: "Rule", Category: CategoryNaming, Level: LevelCheck, Check: &RuleCheck{Type: CheckPathPattern, Target: "[", Pattern: "ok"}}}},
	}
	for _, input := range tests {
		if _, err := service.CreatePackage(context.Background(), input, "admin"); !errors.Is(err, ErrInvalidPackage) {
			t.Fatalf("expected invalid package, got %v for %#v", err, input)
		}
	}
}

func TestGoServiceExamplesRemainExecutable(t *testing.T) {
	packageData, err := os.ReadFile("../../standards/go-service.package.example.json")
	if err != nil {
		t.Fatalf("read package example: %v", err)
	}
	var packageInput CreatePackageInput
	if err := json.Unmarshal(packageData, &packageInput); err != nil {
		t.Fatalf("decode package example: %v", err)
	}
	validationData, err := os.ReadFile("../../standards/go-service.validation.example.json")
	if err != nil {
		t.Fatalf("read validation example: %v", err)
	}
	var validationInput ValidateInput
	if err := json.Unmarshal(validationData, &validationInput); err != nil {
		t.Fatalf("decode validation example: %v", err)
	}
	service := NewService(NewMemoryStore())
	if _, err := service.CreatePackage(context.Background(), packageInput, "example-admin"); err != nil {
		t.Fatalf("publish package example: %v", err)
	}
	report, err := service.Validate(context.Background(), validationInput, "example-developer")
	if err != nil || !report.Passed || report.ViolationCount != 0 {
		t.Fatalf("example must pass: %#v err=%v", report, err)
	}
}
