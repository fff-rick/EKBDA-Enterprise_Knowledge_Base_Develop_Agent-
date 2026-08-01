package initiative

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"testing"
	"time"
)

func TestRenderDOCXHasValidRequiredParts(t *testing.T) {
	projectPackage := Package{
		Project: "order-service", Name: "order-export", Version: 1, Repository: "order-service",
		DefinitionHash: "hash", ChangeSummary: "initial package", CreatedBy: "approver", CreatedAt: time.Unix(0, 0).UTC(),
		Source:       SourceSnapshot{PlanningSessionID: "session-1"},
		Artifacts:    []Artifact{{Type: ArtifactPRD, Title: "Order export PRD", Summary: "Auditable exports", Sections: []Section{{Name: "Goals", Items: []string{"Export orders"}}}}},
		Traceability: []TraceRecord{{RequirementID: "REQ-001", Requirement: "Export orders", ArchitectureSections: []string{"Design"}, APIApplicable: true, APISections: []string{"Contract"}, TestSections: []string{"Acceptance"}, DeploymentSections: []string{"Prerequisites"}, CoverageStatus: "covered"}},
	}
	data, err := RenderDOCX(projectPackage, nil)
	if err != nil {
		t.Fatalf("render DOCX: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open DOCX archive: %v", err)
	}
	required := map[string]bool{
		"[Content_Types].xml": false, "_rels/.rels": false, "word/document.xml": false,
		"word/styles.xml": false, "word/numbering.xml": false, "word/header1.xml": false, "word/footer1.xml": false,
	}
	for _, file := range archive.File {
		if _, found := required[file.Name]; !found {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		decoder := xml.NewDecoder(bytes.NewReader(content))
		for {
			if _, err := decoder.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("parse %s: %v", file.Name, err)
			}
		}
		required[file.Name] = true
	}
	for name, found := range required {
		if !found {
			t.Errorf("missing DOCX part %s", name)
		}
	}
}
