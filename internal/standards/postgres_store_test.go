package standards

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestPostgresStandardsRoundTrip(t *testing.T) {
	dsn := os.Getenv("EKBDA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EKBDA_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewPostgresStore(ctx, dsn)
	if err != nil {
		t.Fatalf("create PostgreSQL standards store: %v", err)
	}
	defer store.Close()
	service := NewService(store)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	project := "standards-" + suffix
	input := CreatePackageInput{
		Name: "project-" + suffix, Scope: ScopeProject, Selector: project, Owner: "integration-test",
		Rules: []Rule{{ID: "readme-" + suffix, Title: "README", Category: CategoryDirectory, Level: LevelBlock, Check: &RuleCheck{Type: CheckRequiredPath, Pattern: "README.md"}}},
	}
	first, err := service.CreatePackage(ctx, input, "integration-admin")
	if err != nil {
		t.Fatalf("create package: %v", err)
	}
	second, err := service.CreatePackage(ctx, input, "integration-admin")
	if err != nil || first.Version != 1 || second.Version != 2 {
		t.Fatalf("create package version: first=%#v second=%#v err=%v", first, second, err)
	}
	report, err := service.Validate(ctx, ValidateInput{
		Project: project, Technology: "go", Files: []File{{Path: "README.md", Content: "# Test"}},
	}, "integration-developer")
	if err != nil || !report.Passed || len(report.Packages) != 1 || report.Packages[0].Version != 2 {
		t.Fatalf("validate standards: %#v err=%v", report, err)
	}
	stored, err := service.GetReport(ctx, report.ID)
	if err != nil || stored.InputHash != report.InputHash || stored.ValidatedBy != "integration-developer" {
		t.Fatalf("get report: %#v err=%v", stored, err)
	}
}
