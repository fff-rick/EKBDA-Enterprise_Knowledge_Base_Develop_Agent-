package initiative

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ekbda/internal/planning"
)

var packageKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var requirementIDPattern = regexp.MustCompile(`^REQ-[0-9]{3}$`)

type Service struct {
	store    Store
	provider Provider
	planning *planning.Service
}

func NewService(store Store, provider Provider, planningService *planning.Service) *Service {
	return &Service{store: store, provider: provider, planning: planningService}
}

func (s *Service) Create(ctx context.Context, input CreateInput, actor string) (Package, error) {
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.Name = strings.ToLower(strings.TrimSpace(input.Name))
	input.ChangeSummary = strings.TrimSpace(input.ChangeSummary)
	actor = strings.TrimSpace(actor)
	if input.SessionID == "" || !packageKeyPattern.MatchString(input.Name) || input.ChangeSummary == "" || len(input.ChangeSummary) > 2000 || !utf8.ValidString(input.ChangeSummary) || actor == "" || len(actor) > 256 {
		return Package{}, ErrInvalidInput
	}
	session, err := s.planning.Get(ctx, input.SessionID)
	if err != nil {
		return Package{}, err
	}
	if !approvedPlanningSource(session) {
		return Package{}, ErrPlanningNotApproved
	}
	output, err := s.provider.Build(ctx, session)
	if err != nil {
		return Package{}, fmt.Errorf("%w: generate project package: %v", ErrProviderFailed, err)
	}
	artifacts, err := validateAndOrderArtifacts(output.Artifacts, session)
	if err != nil {
		return Package{}, err
	}
	traceability, err := validateTraceability(output.Traceability, artifacts)
	if err != nil {
		return Package{}, err
	}
	now := time.Now().UTC()
	projectPackage := Package{
		ID: newID(), Project: session.Project, Repository: session.Repository, Name: input.Name,
		ChangeSummary: input.ChangeSummary, Provider: s.provider.Name(), Artifacts: artifacts, Traceability: traceability,
		Source: SourceSnapshot{
			PlanningSessionID: session.ID, PlanningRevision: session.Revision,
			PlanContextHash: session.Context.Hash, ReviewContextHash: session.RoleReview.Context.Hash,
			PlanApprovedBy: session.ReviewedBy, PlanApprovedAt: cloneTime(session.ReviewedAt),
		},
		CreatedBy: actor, CreatedAt: now,
	}
	definition, err := json.Marshal(struct {
		Project       string         `json:"project"`
		Repository    string         `json:"repository"`
		Name          string         `json:"name"`
		ChangeSummary string         `json:"change_summary"`
		Provider      string         `json:"provider"`
		Source        SourceSnapshot `json:"source"`
		Artifacts     []Artifact     `json:"artifacts"`
		Traceability  []TraceRecord  `json:"traceability"`
	}{projectPackage.Project, projectPackage.Repository, projectPackage.Name, projectPackage.ChangeSummary, projectPackage.Provider, projectPackage.Source, projectPackage.Artifacts, projectPackage.Traceability})
	if err != nil {
		return Package{}, fmt.Errorf("encode project package definition: %w", err)
	}
	digest := sha256.Sum256(definition)
	projectPackage.DefinitionHash = hex.EncodeToString(digest[:])
	return s.store.Create(ctx, projectPackage)
}

func validateTraceability(records []TraceRecord, artifacts []Artifact) ([]TraceRecord, error) {
	byType := make(map[string]Artifact, len(artifacts))
	for _, artifact := range artifacts {
		byType[artifact.Type] = artifact
	}
	requirements := make(map[string]struct{})
	for _, section := range byType[ArtifactPRD].Sections {
		for _, item := range section.Items {
			requirements[item] = struct{}{}
		}
	}
	if len(records) != len(requirements) {
		return nil, ErrInvalidProviderOutput
	}
	seenIDs := make(map[string]struct{}, len(records))
	seenRecords := make(map[string]struct{}, len(records))
	for index := range records {
		record := &records[index]
		if !requirementIDPattern.MatchString(record.RequirementID) || record.RequirementID != fmt.Sprintf("REQ-%03d", index+1) {
			return nil, ErrInvalidProviderOutput
		}
		if _, found := seenIDs[record.RequirementID]; found {
			return nil, ErrInvalidProviderOutput
		}
		seenIDs[record.RequirementID] = struct{}{}
		if _, found := requirements[record.Requirement]; !found {
			return nil, ErrInvalidProviderOutput
		}
		if _, found := seenRecords[record.Requirement]; found {
			return nil, ErrInvalidProviderOutput
		}
		seenRecords[record.Requirement] = struct{}{}
		if !validSectionLinks(record.ArchitectureSections, byType[ArtifactArchitecture]) || !validSectionLinks(record.TestSections, byType[ArtifactTest]) || !validSectionLinks(record.DeploymentSections, byType[ArtifactDeployment]) {
			return nil, ErrInvalidProviderOutput
		}
		if record.APIApplicable {
			if strings.TrimSpace(record.APINotApplicableReason) != "" || !validSectionLinks(record.APISections, byType[ArtifactAPI]) {
				return nil, ErrInvalidProviderOutput
			}
		} else if len(record.APISections) != 0 || strings.TrimSpace(record.APINotApplicableReason) == "" || len(record.APINotApplicableReason) > 1000 || !utf8.ValidString(record.APINotApplicableReason) {
			return nil, ErrInvalidProviderOutput
		}
		record.Gaps = traceGaps(*record)
		if len(record.Gaps) == 0 {
			record.CoverageStatus = "covered"
		} else {
			record.CoverageStatus = "partial"
		}
	}
	return records, nil
}

func validSectionLinks(values []string, artifact Artifact) bool {
	if len(values) > 20 {
		return false
	}
	sections := make(map[string]struct{}, len(artifact.Sections))
	for _, section := range artifact.Sections {
		sections[section.Name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, found := sections[value]; !found {
			return false
		}
		if _, found := seen[value]; found {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func traceGaps(record TraceRecord) []string {
	gaps := make([]string, 0, 4)
	if len(record.ArchitectureSections) == 0 {
		gaps = append(gaps, "architecture")
	}
	if record.APIApplicable && len(record.APISections) == 0 {
		gaps = append(gaps, "api")
	}
	if len(record.TestSections) == 0 {
		gaps = append(gaps, "test")
	}
	if len(record.DeploymentSections) == 0 {
		gaps = append(gaps, "deployment")
	}
	return gaps
}

func approvedPlanningSource(session planning.Session) bool {
	if session.Status != planning.StatusApproved || session.Plan == nil || session.RoleReview == nil || session.Context.Hash == "" || session.RoleReview.Context.Hash == "" || session.ReviewedBy == "" || session.ReviewedAt == nil {
		return false
	}
	requiredRoles := planning.RequiredReviewRoles()
	if len(session.RoleReview.Reviews) != len(requiredRoles) {
		return false
	}
	seenRoles := make(map[string]struct{}, len(requiredRoles))
	for _, review := range session.RoleReview.Reviews {
		seenRoles[review.Role] = struct{}{}
	}
	for _, role := range requiredRoles {
		if _, found := seenRoles[role]; !found {
			return false
		}
	}
	for _, decision := range session.RoleReview.Coordination.DecisionItems {
		if strings.TrimSpace(decision.Resolution) == "" || strings.TrimSpace(decision.ResolvedBy) == "" || decision.ResolvedAt == nil {
			return false
		}
	}
	return true
}

func (s *Service) Get(ctx context.Context, id string) (Package, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, project, name string, limit int) ([]Package, error) {
	project = strings.ToLower(strings.TrimSpace(project))
	name = strings.ToLower(strings.TrimSpace(name))
	if !packageKeyPattern.MatchString(project) || (name != "" && !packageKeyPattern.MatchString(name)) {
		return nil, ErrInvalidInput
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.List(ctx, project, name, limit)
}

func (s *Service) Review(ctx context.Context, packageID string, input ReviewInput, actor string) (ArtifactReview, error) {
	packageID = strings.TrimSpace(packageID)
	input.ArtifactType = strings.TrimSpace(input.ArtifactType)
	input.PackageHash = strings.TrimSpace(input.PackageHash)
	input.Decision = strings.TrimSpace(input.Decision)
	input.Comment = strings.TrimSpace(input.Comment)
	actor = strings.TrimSpace(actor)
	if packageID == "" || !requiredArtifactType(input.ArtifactType) || (input.Decision != "approve" && input.Decision != "request_changes") || input.Comment == "" || len(input.Comment) > 4000 || !utf8.ValidString(input.Comment) || actor == "" || len(actor) > 256 {
		return ArtifactReview{}, ErrInvalidReview
	}
	projectPackage, err := s.store.Get(ctx, packageID)
	if err != nil {
		return ArtifactReview{}, err
	}
	if input.PackageHash == "" || input.PackageHash != projectPackage.DefinitionHash {
		return ArtifactReview{}, ErrPackageHashConflict
	}
	review := ArtifactReview{
		ID: newID(), PackageID: packageID, ArtifactType: input.ArtifactType,
		PackageHash: input.PackageHash, Decision: input.Decision, Comment: input.Comment,
		ReviewedBy: actor, ReviewedAt: time.Now().UTC(),
	}
	return s.store.CreateReview(ctx, review)
}

func (s *Service) Reviews(ctx context.Context, packageID, artifactType string, limit int) ([]ArtifactReview, error) {
	packageID = strings.TrimSpace(packageID)
	artifactType = strings.TrimSpace(artifactType)
	if packageID == "" || (artifactType != "" && !requiredArtifactType(artifactType)) {
		return nil, ErrInvalidReview
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.ListReviews(ctx, packageID, artifactType, limit)
}

func validateAndOrderArtifacts(artifacts []Artifact, session planning.Session) ([]Artifact, error) {
	if len(artifacts) != len(requiredArtifactTypes) {
		return nil, ErrInvalidProviderOutput
	}
	allowedReferences := referenceWhitelist(session)
	byType := make(map[string]Artifact, len(artifacts))
	for _, artifact := range artifacts {
		if !requiredArtifactType(artifact.Type) || strings.TrimSpace(artifact.Title) == "" || len(artifact.Title) > 300 || !utf8.ValidString(artifact.Title) || strings.TrimSpace(artifact.Summary) == "" || len(artifact.Summary) > 4000 || !utf8.ValidString(artifact.Summary) || len(artifact.Sections) == 0 || len(artifact.Sections) > 20 || len(artifact.References) > 50 {
			return nil, ErrInvalidProviderOutput
		}
		if _, found := byType[artifact.Type]; found {
			return nil, ErrInvalidProviderOutput
		}
		sectionNames := make(map[string]struct{}, len(artifact.Sections))
		for _, section := range artifact.Sections {
			section.Name = strings.TrimSpace(section.Name)
			if section.Name == "" || len(section.Name) > 300 || !utf8.ValidString(section.Name) || len(section.Items) == 0 || len(section.Items) > 50 || !validTextList(section.Items, 2000) {
				return nil, ErrInvalidProviderOutput
			}
			if _, found := sectionNames[section.Name]; found {
				return nil, ErrInvalidProviderOutput
			}
			sectionNames[section.Name] = struct{}{}
		}
		seenReferences := make(map[string]struct{}, len(artifact.References))
		for _, reference := range artifact.References {
			key := reference.Kind + ":" + reference.ID
			if _, allowed := allowedReferences[key]; !allowed {
				return nil, ErrInvalidProviderOutput
			}
			if _, found := seenReferences[key]; found {
				return nil, ErrInvalidProviderOutput
			}
			seenReferences[key] = struct{}{}
		}
		byType[artifact.Type] = artifact
	}
	ordered := make([]Artifact, 0, len(requiredArtifactTypes))
	for _, artifactType := range requiredArtifactTypes {
		artifact, found := byType[artifactType]
		if !found {
			return nil, ErrInvalidProviderOutput
		}
		ordered = append(ordered, artifact)
	}
	return ordered, nil
}

func referenceWhitelist(session planning.Session) map[string]struct{} {
	result := make(map[string]struct{})
	for _, reference := range session.Context.Knowledge {
		result[ReferencePlanKnowledge+":"+reference.ID] = struct{}{}
	}
	for _, reference := range session.Context.Standards {
		result[ReferencePlanStandard+":"+reference.ID] = struct{}{}
	}
	for _, reference := range session.RoleReview.Context.Knowledge {
		result[ReferenceReviewKnowledge+":"+reference.ID] = struct{}{}
	}
	for _, reference := range session.RoleReview.Context.Standards {
		result[ReferenceReviewStandard+":"+reference.ID] = struct{}{}
	}
	for _, decision := range session.RoleReview.Coordination.DecisionItems {
		if decision.Resolution != "" && decision.ResolvedBy != "" && decision.ResolvedAt != nil {
			result[ReferenceDecision+":"+decision.ID] = struct{}{}
		}
	}
	return result
}

func requiredArtifactType(value string) bool {
	for _, expected := range requiredArtifactTypes {
		if value == expected {
			return true
		}
	}
	return false
}

func validTextList(values []string, maxLength int) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > maxLength || !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("package-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
