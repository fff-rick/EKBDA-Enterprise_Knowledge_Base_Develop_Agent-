package agentquality

import (
	"fmt"

	"ekbda/internal/agenttask"
	"ekbda/internal/initiative"
	"ekbda/internal/planning"
)

func RoleReview(session planning.Session) agenttask.QualityReport {
	checks := []agenttask.QualityCheck{
		{Name: "role_review_cycle", Passed: session.RoleReview != nil},
		{Name: "terminal_review_status", Passed: session.Status == planning.StatusAwaitingResolution || session.Status == planning.StatusAwaitingApproval, Details: session.Status},
	}
	if session.RoleReview != nil {
		seen := make(map[string]struct{}, len(session.RoleReview.Reviews))
		for _, review := range session.RoleReview.Reviews {
			seen[review.Role] = struct{}{}
		}
		complete := len(seen) == len(planning.RequiredReviewRoles())
		for _, role := range planning.RequiredReviewRoles() {
			if _, found := seen[role]; !found {
				complete = false
			}
		}
		checks = append(checks,
			agenttask.QualityCheck{Name: "required_roles", Passed: complete, Details: fmt.Sprintf("%d/%d", len(seen), len(planning.RequiredReviewRoles()))},
			agenttask.QualityCheck{Name: "review_context_snapshot", Passed: session.RoleReview.Context.Hash != ""},
		)
	}
	return report(checks)
}

func ProjectPackage(projectPackage initiative.Package) agenttask.QualityReport {
	artifactTypes := make(map[string]struct{}, len(projectPackage.Artifacts))
	for _, artifact := range projectPackage.Artifacts {
		artifactTypes[artifact.Type] = struct{}{}
	}
	artifactsComplete := len(artifactTypes) == len(initiative.RequiredArtifactTypes())
	for _, artifactType := range initiative.RequiredArtifactTypes() {
		if _, found := artifactTypes[artifactType]; !found {
			artifactsComplete = false
		}
	}
	covered := len(projectPackage.Traceability) > 0
	partialCount := 0
	for _, trace := range projectPackage.Traceability {
		if trace.CoverageStatus != "covered" {
			covered = false
			partialCount++
		}
	}
	checks := []agenttask.QualityCheck{
		{Name: "required_artifacts", Passed: artifactsComplete, Details: fmt.Sprintf("%d/%d", len(artifactTypes), len(initiative.RequiredArtifactTypes()))},
		{Name: "traceability_present", Passed: len(projectPackage.Traceability) > 0, Details: fmt.Sprintf("%d records", len(projectPackage.Traceability))},
		{Name: "traceability_covered", Passed: covered, Details: fmt.Sprintf("%d partial records", partialCount)},
		{Name: "source_snapshot", Passed: projectPackage.Source.PlanningSessionID != "" && projectPackage.Source.PlanContextHash != "" && projectPackage.Source.ReviewContextHash != ""},
		{Name: "definition_hash", Passed: projectPackage.DefinitionHash != ""},
	}
	return report(checks)
}

func report(checks []agenttask.QualityCheck) agenttask.QualityReport {
	passed := true
	for _, check := range checks {
		if !check.Passed {
			passed = false
		}
	}
	return agenttask.QualityReport{Passed: passed, Checks: checks}
}
