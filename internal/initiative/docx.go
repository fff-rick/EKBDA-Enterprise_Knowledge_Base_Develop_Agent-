package initiative

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"strings"
	"time"
)

func RenderDOCX(projectPackage Package, reviews []ArtifactReview) ([]byte, error) {
	document := renderDocumentXML(projectPackage, reviews)
	createdAt := projectPackage.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Unix(0, 0).UTC()
	}
	parts := []struct {
		name string
		data string
	}{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", rootRelationshipsXML},
		{"docProps/app.xml", appPropertiesXML},
		{"docProps/core.xml", corePropertiesXML(projectPackage, createdAt)},
		{"word/document.xml", document},
		{"word/styles.xml", stylesXML},
		{"word/numbering.xml", numberingXML},
		{"word/settings.xml", settingsXML},
		{"word/header1.xml", headerXML(projectPackage)},
		{"word/footer1.xml", footerXML},
		{"word/_rels/document.xml.rels", documentRelationshipsXML},
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, part := range parts {
		header := &zip.FileHeader{Name: part.name, Method: zip.Deflate}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(part.data)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func renderDocumentXML(projectPackage Package, reviews []ArtifactReview) string {
	var body strings.Builder
	body.WriteString(docxParagraph("Title", "项目立项包", ""))
	body.WriteString(docxParagraph("Subtitle", fmt.Sprintf("%s / %s · v%d", projectPackage.Project, projectPackage.Name, projectPackage.Version), ""))
	body.WriteString(`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="2E74B5"/></w:pBdr><w:spacing w:after="120"/></w:pPr></w:p>`)
	body.WriteString(docxLabelParagraph("Repository", projectPackage.Repository))
	body.WriteString(docxLabelParagraph("Change summary", projectPackage.ChangeSummary))
	body.WriteString(docxLabelParagraph("Definition hash", projectPackage.DefinitionHash))
	body.WriteString(docxLabelParagraph("Planning session", projectPackage.Source.PlanningSessionID))
	body.WriteString(docxLabelParagraph("Created", projectPackage.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")+" by "+projectPackage.CreatedBy))

	body.WriteString(docxParagraph("Heading1", "追踪矩阵", ""))
	traceRows := make([][]string, 0, len(projectPackage.Traceability))
	for _, record := range projectPackage.Traceability {
		api := strings.Join(record.APISections, "\n")
		if !record.APIApplicable {
			api = "N/A\n" + record.APINotApplicableReason
		}
		status := record.CoverageStatus
		if len(record.Gaps) > 0 {
			status += "\nGaps: " + strings.Join(record.Gaps, ", ")
		}
		traceRows = append(traceRows, []string{
			record.RequirementID, record.Requirement, strings.Join(record.ArchitectureSections, "\n"), api,
			strings.Join(record.TestSections, "\n"), strings.Join(record.DeploymentSections, "\n"), status,
		})
	}
	body.WriteString(docxTable(
		[]string{"ID", "Requirement", "Architecture", "API", "Test", "Deployment", "Status"},
		[]int{720, 2300, 1250, 1250, 1250, 1250, 1340}, traceRows,
	))

	body.WriteString(docxParagraph("Heading1", "交付物", ""))
	for _, artifact := range projectPackage.Artifacts {
		body.WriteString(docxParagraph("Heading1", strings.ToUpper(artifact.Type)+" · "+artifact.Title, ""))
		body.WriteString(docxParagraph("Normal", artifact.Summary, ""))
		for _, section := range artifact.Sections {
			body.WriteString(docxParagraph("Heading2", section.Name, ""))
			for _, item := range section.Items {
				body.WriteString(docxParagraph("Normal", item, `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr>`))
			}
		}
		if len(artifact.References) > 0 {
			body.WriteString(docxParagraph("Heading3", "References", ""))
			references := make([]string, 0, len(artifact.References))
			for _, reference := range artifact.References {
				references = append(references, reference.Kind+":"+reference.ID)
			}
			body.WriteString(docxParagraph("ReferenceText", strings.Join(references, ", "), ""))
		}
	}

	body.WriteString(docxParagraph("Heading1", "产物评审记录", ""))
	if len(reviews) == 0 {
		body.WriteString(docxParagraph("Normal", "No artifact reviews recorded.", ""))
	} else {
		rows := make([][]string, 0, len(reviews))
		for _, review := range reviews {
			rows = append(rows, []string{review.ArtifactType, fmt.Sprintf("%d", review.Sequence), review.Decision, review.ReviewedBy, review.ReviewedAt.UTC().Format("2006-01-02 15:04 UTC"), review.Comment})
		}
		body.WriteString(docxTable([]string{"Artifact", "Seq", "Decision", "Reviewer", "Reviewed at", "Comment"}, []int{1200, 600, 1300, 1300, 1900, 3060}, rows))
	}

	body.WriteString(`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/><w:footerReference w:type="default" r:id="rId2"/><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="708" w:footer="708" w:gutter="0"/><w:cols w:space="708"/><w:docGrid w:linePitch="312"/></w:sectPr>`)
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>` + body.String() + `</w:body></w:document>`
}

func docxParagraph(style, value, extraProperties string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/>` + extraProperties + `</w:pPr><w:r><w:t xml:space="preserve">` + xmlText(value) + `</w:t></w:r></w:p>`
}

func docxLabelParagraph(label, value string) string {
	return `<w:p><w:pPr><w:pStyle w:val="Normal"/></w:pPr><w:r><w:rPr><w:rStyle w:val="MetadataLabel"/></w:rPr><w:t>` + xmlText(label+" · ") + `</w:t></w:r><w:r><w:t xml:space="preserve">` + xmlText(value) + `</w:t></w:r></w:p>`
}

func docxTable(headers []string, widths []int, rows [][]string) string {
	var result strings.Builder
	result.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9360" w:type="dxa"/><w:tblInd w:w="120" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblCellMar><w:top w:w="80" w:type="dxa"/><w:start w:w="120" w:type="dxa"/><w:bottom w:w="80" w:type="dxa"/><w:end w:w="120" w:type="dxa"/></w:tblCellMar><w:tblBorders><w:top w:val="single" w:sz="4" w:color="B7C9DD"/><w:left w:val="single" w:sz="4" w:color="B7C9DD"/><w:bottom w:val="single" w:sz="4" w:color="B7C9DD"/><w:right w:val="single" w:sz="4" w:color="B7C9DD"/><w:insideH w:val="single" w:sz="4" w:color="D9E2F3"/><w:insideV w:val="single" w:sz="4" w:color="D9E2F3"/></w:tblBorders></w:tblPr><w:tblGrid>`)
	for _, width := range widths {
		result.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, width))
	}
	result.WriteString(`</w:tblGrid><w:tr><w:trPr><w:tblHeader/></w:trPr>`)
	for index, value := range headers {
		result.WriteString(docxCell(value, widths[index], true))
	}
	result.WriteString(`</w:tr>`)
	for _, row := range rows {
		result.WriteString(`<w:tr>`)
		for index, value := range row {
			result.WriteString(docxCell(value, widths[index], false))
		}
		result.WriteString(`</w:tr>`)
	}
	result.WriteString(`</w:tbl>`)
	return result.String()
}

func docxCell(value string, width int, header bool) string {
	shade := ""
	style := "TableText"
	if header {
		shade = `<w:shd w:val="clear" w:color="auto" w:fill="E8EEF5"/>`
		style = "TableHeader"
	}
	var paragraphs strings.Builder
	for _, line := range strings.Split(value, "\n") {
		paragraphs.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr><w:r><w:t xml:space="preserve">` + xmlText(line) + `</w:t></w:r></w:p>`)
	}
	return fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s<w:vAlign w:val="top"/></w:tcPr>%s</w:tc>`, width, shade, paragraphs.String())
}

func xmlText(value string) string { return html.EscapeString(value) }

func corePropertiesXML(projectPackage Package, createdAt time.Time) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>` + xmlText(projectPackage.Name) + ` project package</dc:title><dc:subject>Enterprise project package</dc:subject><dc:creator></dc:creator><cp:keywords>project package;traceability;review</cp:keywords><dc:description>` + xmlText(projectPackage.ChangeSummary) + `</dc:description><dcterms:created xsi:type="dcterms:W3CDTF">` + createdAt.Format(time.RFC3339) + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + createdAt.Format(time.RFC3339) + `</dcterms:modified></cp:coreProperties>`
}

func headerXML(projectPackage Package) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="right"/><w:spacing w:after="0"/></w:pPr><w:r><w:rPr><w:color w:val="6B7280"/><w:sz w:val="16"/></w:rPr><w:t>` + xmlText(fmt.Sprintf("EKBDA · %s / %s · v%d", projectPackage.Project, projectPackage.Name, projectPackage.Version)) + `</w:t></w:r></w:p></w:hdr>`
}

const footerXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:color w:val="6B7280"/><w:sz w:val="16"/></w:rPr><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p></w:ftr>`

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/><Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/><Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/><Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`

const rootRelationshipsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`

const documentRelationshipsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/><Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/></Relationships>`

const appPropertiesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>EKBDA</Application><DocSecurity>0</DocSecurity><ScaleCrop>false</ScaleCrop><Company></Company><LinksUpToDate>false</LinksUpToDate><SharedDoc>false</SharedDoc><HyperlinksChanged>false</HyperlinksChanged><AppVersion>1.0</AppVersion></Properties>`

const settingsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:zoom w:percent="100"/><w:defaultTabStop w:val="720"/><w:characterSpacingControl w:val="doNotCompress"/><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat></w:settings>`

const numberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="singleLevel"/><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/><w:pPr><w:tabs><w:tab w:val="num" w:pos="540"/></w:tabs><w:ind w:left="540" w:hanging="270"/></w:pPr><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/></w:rPr></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num></w:numbering>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Microsoft YaHei"/><w:sz w:val="22"/><w:szCs w:val="22"/><w:lang w:val="en-US" w:eastAsia="zh-CN"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="300" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/><w:pPr><w:spacing w:after="120" w:line="300" w:lineRule="auto"/></w:pPr></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:next w:val="Subtitle"/><w:qFormat/><w:pPr><w:spacing w:before="0" w:after="80"/><w:keepNext/></w:pPr><w:rPr><w:b/><w:sz w:val="48"/><w:szCs w:val="48"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:spacing w:before="0" w:after="120"/><w:keepNext/></w:pPr><w:rPr><w:color w:val="6B7280"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="360" w:after="200"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:color w:val="2E74B5"/><w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="280" w:after="140"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/><w:sz w:val="26"/><w:szCs w:val="26"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="200" w:after="100"/><w:outlineLvl w:val="2"/></w:pPr><w:rPr><w:b/><w:sz w:val="24"/><w:szCs w:val="24"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="ReferenceText"><w:name w:val="Reference Text"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="120" w:line="276" w:lineRule="auto"/></w:pPr><w:rPr><w:color w:val="6B7280"/><w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="TableText"><w:name w:val="Table Text"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="80" w:line="300" w:lineRule="auto"/></w:pPr><w:rPr><w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="TableHeader"><w:name w:val="Table Header"/><w:basedOn w:val="TableText"/><w:rPr><w:b/></w:rPr></w:style><w:style w:type="character" w:styleId="MetadataLabel"><w:name w:val="Metadata Label"/><w:rPr><w:b/><w:color w:val="2E74B5"/></w:rPr></w:style></w:styles>`
