package agentquality

import (
	"testing"

	"ekbda/internal/initiative"
	"ekbda/internal/planning"
)

func TestRoleReviewQualityRequiresEveryRole(t *testing.T) {
	reviews := make([]planning.RoleReview, 0, len(planning.RequiredReviewRoles()))
	for _, role := range planning.RequiredReviewRoles() {
		reviews = append(reviews, planning.RoleReview{Role: role})
	}
	session := planning.Session{
		Status: planning.StatusAwaitingApproval,
		RoleReview: &planning.RoleReviewCycle{
			Context: planning.ContextSnapshot{Hash: "review-context"}, Reviews: reviews,
		},
	}
	if report := RoleReview(session); !report.Passed {
		t.Fatalf("complete role review must pass: %#v", report)
	}
	session.RoleReview.Reviews = session.RoleReview.Reviews[:len(session.RoleReview.Reviews)-1]
	if report := RoleReview(session); report.Passed {
		t.Fatalf("incomplete role review must fail: %#v", report)
	}
}

func TestProjectPackageQualityRejectsPartialTraceability(t *testing.T) {
	artifacts := make([]initiative.Artifact, 0, len(initiative.RequiredArtifactTypes()))
	for _, artifactType := range initiative.RequiredArtifactTypes() {
		artifacts = append(artifacts, initiative.Artifact{Type: artifactType})
	}
	projectPackage := initiative.Package{
		DefinitionHash: "hash", Artifacts: artifacts,
		Source:       initiative.SourceSnapshot{PlanningSessionID: "session", PlanContextHash: "plan", ReviewContextHash: "review"},
		Traceability: []initiative.TraceRecord{{RequirementID: "REQ-001", CoverageStatus: "partial"}},
	}
	if report := ProjectPackage(projectPackage); report.Passed {
		t.Fatalf("partial traceability must fail: %#v", report)
	}
	projectPackage.Traceability[0].CoverageStatus = "covered"
	if report := ProjectPackage(projectPackage); !report.Passed {
		t.Fatalf("covered package must pass: %#v", report)
	}
}
