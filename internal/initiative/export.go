package initiative

import (
	"context"
	"fmt"
	"strings"
)

const (
	ExportMarkdown = "markdown"
	ExportDOCX     = "docx"
)

func (s *Service) Export(ctx context.Context, packageID, format string) (ExportedDocument, error) {
	projectPackage, err := s.Get(ctx, packageID)
	if err != nil {
		return ExportedDocument{}, err
	}
	reviews, err := s.Reviews(ctx, packageID, "", 200)
	if err != nil {
		return ExportedDocument{}, err
	}
	baseName := fmt.Sprintf("%s-%s-v%d", projectPackage.Project, projectPackage.Name, projectPackage.Version)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case ExportMarkdown:
		return ExportedDocument{Filename: baseName + ".md", ContentType: "text/markdown; charset=utf-8", Data: RenderMarkdown(projectPackage, reviews)}, nil
	case ExportDOCX:
		data, err := RenderDOCX(projectPackage, reviews)
		if err != nil {
			return ExportedDocument{}, fmt.Errorf("render project package DOCX: %w", err)
		}
		return ExportedDocument{Filename: baseName + ".docx", ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Data: data}, nil
	default:
		return ExportedDocument{}, ErrInvalidInput
	}
}

func RenderMarkdown(projectPackage Package, reviews []ArtifactReview) []byte {
	var result strings.Builder
	result.WriteString("# Project Package: ")
	result.WriteString(projectPackage.Name)
	result.WriteString("\n\n")
	writeMarkdownField(&result, "Project", projectPackage.Project)
	writeMarkdownField(&result, "Repository", projectPackage.Repository)
	writeMarkdownField(&result, "Version", fmt.Sprintf("%d", projectPackage.Version))
	writeMarkdownField(&result, "Definition hash", projectPackage.DefinitionHash)
	writeMarkdownField(&result, "Change summary", projectPackage.ChangeSummary)
	writeMarkdownField(&result, "Provider", projectPackage.Provider)
	writeMarkdownField(&result, "Planning session", projectPackage.Source.PlanningSessionID)
	writeMarkdownField(&result, "Created by", projectPackage.CreatedBy)
	writeMarkdownField(&result, "Created at", projectPackage.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))

	result.WriteString("\n## Traceability matrix\n\n")
	result.WriteString("| ID | Requirement | Architecture | API | Test | Deployment | Status |\n")
	result.WriteString("|---|---|---|---|---|---|---|\n")
	for _, record := range projectPackage.Traceability {
		api := strings.Join(record.APISections, ", ")
		if !record.APIApplicable {
			api = "N/A: " + record.APINotApplicableReason
		}
		values := []string{record.RequirementID, record.Requirement, strings.Join(record.ArchitectureSections, ", "), api, strings.Join(record.TestSections, ", "), strings.Join(record.DeploymentSections, ", "), record.CoverageStatus}
		result.WriteString("|")
		for _, value := range values {
			result.WriteString(" ")
			result.WriteString(markdownCell(value))
			result.WriteString(" |")
		}
		result.WriteString("\n")
	}

	result.WriteString("\n## Artifacts\n")
	for _, artifact := range projectPackage.Artifacts {
		result.WriteString("\n### ")
		result.WriteString(artifact.Type)
		result.WriteString(": ")
		result.WriteString(artifact.Title)
		result.WriteString("\n\n")
		result.WriteString(artifact.Summary)
		result.WriteString("\n")
		for _, section := range artifact.Sections {
			result.WriteString("\n#### ")
			result.WriteString(section.Name)
			result.WriteString("\n\n")
			for _, item := range section.Items {
				result.WriteString("- ")
				result.WriteString(strings.ReplaceAll(item, "\n", " "))
				result.WriteString("\n")
			}
		}
		if len(artifact.References) > 0 {
			result.WriteString("\nReferences: ")
			for index, reference := range artifact.References {
				if index > 0 {
					result.WriteString(", ")
				}
				result.WriteString(reference.Kind + ":" + reference.ID)
			}
			result.WriteString("\n")
		}
	}

	result.WriteString("\n## Artifact reviews\n\n")
	if len(reviews) == 0 {
		result.WriteString("No artifact reviews recorded.\n")
	} else {
		result.WriteString("| Artifact | Sequence | Decision | Reviewer | Reviewed at | Comment |\n")
		result.WriteString("|---|---:|---|---|---|---|\n")
		for _, review := range reviews {
			values := []string{review.ArtifactType, fmt.Sprintf("%d", review.Sequence), review.Decision, review.ReviewedBy, review.ReviewedAt.UTC().Format("2006-01-02T15:04:05Z"), review.Comment}
			result.WriteString("|")
			for _, value := range values {
				result.WriteString(" ")
				result.WriteString(markdownCell(value))
				result.WriteString(" |")
			}
			result.WriteString("\n")
		}
	}
	return []byte(result.String())
}

func writeMarkdownField(result *strings.Builder, label, value string) {
	result.WriteString("- **")
	result.WriteString(label)
	result.WriteString(":** ")
	result.WriteString(strings.ReplaceAll(value, "\n", " "))
	result.WriteString("\n")
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", "<br>")
}
