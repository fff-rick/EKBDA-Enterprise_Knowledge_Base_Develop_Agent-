package release

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	keyPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	hashPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	eventPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

type Service struct {
	store        Store
	development  DevelopmentReader
	connector    Connector
	pipelines    map[string]bool
	environments map[string]bool
	now          func() time.Time
}

func NewService(store Store, development DevelopmentReader, connector Connector, pipelines, environments []string) (*Service, error) {
	if store == nil || development == nil || connector == nil {
		return nil, ErrInvalidInput
	}
	service := &Service{store: store, development: development, connector: connector, pipelines: normalizeCatalog(pipelines), environments: normalizeCatalog(environments), now: func() time.Time { return time.Now().UTC() }}
	if connector.Enabled() && (len(service.pipelines) == 0 || len(service.environments) == 0) {
		return nil, ErrInvalidInput
	}
	return service, nil
}

func normalizeCatalog(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if keyPattern.MatchString(value) {
			result[value] = true
		}
	}
	return result
}

func (s *Service) Enabled() bool { return s != nil && s.connector.Enabled() }

func (s *Service) Catalog() map[string]any {
	pipelines, environments := mapKeys(s.pipelines), mapKeys(s.environments)
	return map[string]any{"enabled": s.Enabled(), "pipelines": pipelines, "environments": environments, "required_checks": append([]string(nil), RequiredChecks...)}
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func (s *Service) Create(ctx context.Context, input CreateInput, actor string) (Request, error) {
	actor = strings.TrimSpace(actor)
	input.DevelopmentSessionID = strings.TrimSpace(input.DevelopmentSessionID)
	input.Environment = strings.ToLower(strings.TrimSpace(input.Environment))
	input.Pipeline = strings.ToLower(strings.TrimSpace(input.Pipeline))
	input.ChangeTicket = strings.TrimSpace(input.ChangeTicket)
	input.ManifestSHA256 = strings.ToLower(strings.TrimSpace(input.ManifestSHA256))
	input.ConfigurationSHA256 = strings.ToLower(strings.TrimSpace(input.ConfigurationSHA256))
	input.RollbackPlan = strings.TrimSpace(input.RollbackPlan)
	input.PromoteFromReleaseID = strings.TrimSpace(input.PromoteFromReleaseID)
	if actor == "" || input.DevelopmentSessionID == "" || !s.environments[input.Environment] || !s.pipelines[input.Pipeline] || len(input.ChangeTicket) < 3 || len(input.ChangeTicket) > 128 || !hashPattern.MatchString(input.ManifestSHA256) || !hashPattern.MatchString(input.ConfigurationSHA256) || len(input.RollbackPlan) < 10 || len(input.RollbackPlan) > 4000 {
		return Request{}, ErrInvalidInput
	}
	session, err := s.development.Get(ctx, input.DevelopmentSessionID)
	if err != nil {
		return Request{}, err
	}
	if session.Status != "delivered" || session.DeliveryStatus != "passed" || !commitPattern.MatchString(session.Commit) || !validHTTPSURL(session.PullRequestURL) {
		return Request{}, ErrDevelopmentNotReady
	}
	now := s.now()
	request := Request{ID: newID(), DevelopmentSessionID: session.ID, Project: strings.ToLower(strings.TrimSpace(session.Project)), Repository: strings.TrimSpace(session.Repository), SourceCommit: session.Commit, PullRequestURL: session.PullRequestURL, Environment: input.Environment, Pipeline: input.Pipeline, ChangeTicket: input.ChangeTicket, ManifestSHA256: input.ManifestSHA256, ConfigurationSHA256: input.ConfigurationSHA256, RollbackPlan: input.RollbackPlan, RequiredChecks: append([]string(nil), RequiredChecks...), Status: StatusAwaitingSourceVerification, Revision: 1, CreatedBy: actor, CreatedAt: now, UpdatedAt: now}
	if !keyPattern.MatchString(request.Project) || request.Repository == "" {
		return Request{}, ErrInvalidInput
	}
	if request.Environment == "production" {
		promoted, err := s.store.Get(ctx, input.PromoteFromReleaseID)
		if err != nil || promoted.Project != request.Project || promoted.SourceCommit != request.SourceCommit || promoted.Environment == "production" || promoted.Status != StatusSucceeded || promoted.Artifact == nil || !validEvidence(promoted.Artifact, promoted.RequiredChecks, promoted.Checks) {
			return Request{}, ErrInvalidInput
		}
		request.PromotedFromReleaseID = promoted.ID
		request.PromotedArtifactDigest = promoted.Artifact.Digest
	}
	event := newEvent(request, "created", "", actor, "release request created", now)
	if err := s.store.Create(ctx, request, event); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (s *Service) ReconcileCodePlatform(ctx context.Context, source CodePlatformEvent, payload []byte) (Request, bool, error) {
	source.EventID, source.ReleaseID = strings.TrimSpace(source.EventID), strings.TrimSpace(source.ReleaseID)
	source.PullRequestURL = strings.TrimSpace(source.PullRequestURL)
	source.HeadCommit, source.MergeCommit = strings.ToLower(strings.TrimSpace(source.HeadCommit)), strings.ToLower(strings.TrimSpace(source.MergeCommit))
	if !eventPattern.MatchString(source.EventID) || !eventPattern.MatchString(source.ReleaseID) || len(payload) == 0 {
		return Request{}, false, ErrInvalidInput
	}
	payloadHash := sha256.Sum256(payload)
	for attempt := 0; attempt < 3; attempt++ {
		request, err := s.store.Get(ctx, source.ReleaseID)
		if err != nil {
			return Request{}, false, err
		}
		if request.PullRequestURL != source.PullRequestURL || request.SourceCommit != source.HeadCommit {
			return Request{}, false, ErrProviderConflict
		}
		previous, now := request.Status, s.now()
		request.Revision++
		request.UpdatedAt = now
		eventType, reason := "code_platform_event_ignored", "source promotion conditions are not complete"
		if request.Status == StatusAwaitingSourceVerification && source.Merged && source.ProtectedBranch && source.ChecksPassed && source.RequiredApprovals > 0 && source.Approvals >= source.RequiredApprovals && commitPattern.MatchString(source.MergeCommit) {
			request.Status = StatusAwaitingApproval
			request.Commit = source.MergeCommit
			request.Source = &SourceEvidence{EventID: source.EventID, HeadCommit: source.HeadCommit, MergeCommit: source.MergeCommit, ProtectedBranch: true, Approvals: source.Approvals, RequiredApprovals: source.RequiredApprovals, ChecksPassed: true, VerifiedAt: now}
			request.TriggerConfirmation = confirmation(request)
			eventType, reason = "code_platform_verified", "protected branch merge, approvals and required checks verified"
		}
		event := newEvent(request, eventType, previous, "code-platform", reason, now)
		event.ProviderEventID = source.EventID
		applied, err := s.store.ApplyProviderEvent(ctx, request, request.Revision-1, event, source.EventID, hex.EncodeToString(payloadHash[:]))
		if errors.Is(err, ErrRevisionConflict) {
			continue
		}
		if err != nil {
			return Request{}, false, err
		}
		if !applied {
			current, getErr := s.store.Get(ctx, source.ReleaseID)
			return current, false, getErr
		}
		return request, true, nil
	}
	return Request{}, false, ErrRevisionConflict
}

func (s *Service) Decide(ctx context.Context, id string, input DecisionInput, actor string) (Request, error) {
	request, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Request{}, err
	}
	actor, input.Decision, input.Comment = strings.TrimSpace(actor), strings.ToLower(strings.TrimSpace(input.Decision)), strings.TrimSpace(input.Comment)
	if actor == "" || input.Revision < 1 || len(input.Comment) > 2000 || (input.Decision != "approve" && input.Decision != "reject") {
		return Request{}, ErrInvalidInput
	}
	if request.Revision != input.Revision {
		return Request{}, ErrRevisionConflict
	}
	if request.Status != StatusAwaitingApproval {
		return Request{}, ErrInvalidTransition
	}
	if actor == request.CreatedBy {
		return Request{}, ErrSelfApproval
	}
	previous, now := request.Status, s.now()
	request.Revision++
	request.UpdatedAt = now
	request.ApprovalComment = input.Comment
	if input.Decision == "approve" {
		request.Status = StatusApproved
		request.ApprovedBy = actor
		request.ApprovedAt = &now
	} else {
		request.Status = StatusRejected
	}
	if err := s.store.Update(ctx, request, input.Revision, newEvent(request, input.Decision+"d", previous, actor, input.Comment, now)); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (s *Service) Trigger(ctx context.Context, id string, input TriggerInput, actor string) (Request, error) {
	if !s.Enabled() {
		return Request{}, ErrDisabled
	}
	request, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Request{}, err
	}
	if strings.TrimSpace(actor) == "" || input.Revision < 1 {
		return Request{}, ErrInvalidInput
	}
	if request.Revision != input.Revision {
		return Request{}, ErrRevisionConflict
	}
	if request.Status != StatusApproved {
		return Request{}, ErrInvalidTransition
	}
	if input.Confirmation != request.TriggerConfirmation {
		return Request{}, ErrConfirmation
	}
	run, err := s.connector.Trigger(ctx, TriggerRequest{ReleaseID: request.ID, Project: request.Project, Repository: request.Repository, Commit: request.Commit, PullRequestURL: request.PullRequestURL, Environment: request.Environment, Pipeline: request.Pipeline, ChangeTicket: request.ChangeTicket, ManifestSHA256: request.ManifestSHA256, ConfigurationSHA256: request.ConfigurationSHA256, RequiredChecks: request.RequiredChecks, ExpectedArtifactDigest: request.PromotedArtifactDigest, RequireArtifactSignature: true, RequireProvenance: true, RequireSBOM: true})
	if err != nil {
		return Request{}, fmt.Errorf("%w: %v", ErrProviderRejected, err)
	}
	if !eventPattern.MatchString(run.ID) || !validHTTPSURL(run.URL) {
		return Request{}, ErrProviderRejected
	}
	previous, now := request.Status, s.now()
	request.Status = StatusQueued
	request.Revision++
	request.UpdatedAt = now
	request.RunID = run.ID
	request.RunURL = run.URL
	if err := s.store.Update(ctx, request, input.Revision, newEvent(request, "triggered", previous, actor, "controlled CI/CD run queued", now)); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (s *Service) RequestRollback(ctx context.Context, id string, input RollbackInput, actor string) (Request, error) {
	request, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Request{}, err
	}
	actor, input.Reason = strings.TrimSpace(actor), strings.TrimSpace(input.Reason)
	if actor == "" || input.Revision < 1 || len(input.Reason) < 5 || len(input.Reason) > 2000 {
		return Request{}, ErrInvalidInput
	}
	if request.Revision != input.Revision {
		return Request{}, ErrRevisionConflict
	}
	if request.Status != StatusSucceeded {
		return Request{}, ErrInvalidTransition
	}
	previous, now := request.Status, s.now()
	request.Status = StatusRollbackAwaitingApproval
	request.Revision++
	request.UpdatedAt = now
	request.RollbackRequestedBy = actor
	request.RollbackReason = input.Reason
	if err := s.store.Update(ctx, request, input.Revision, newEvent(request, "rollback_requested", previous, actor, input.Reason, now)); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (s *Service) DecideRollback(ctx context.Context, id string, input DecisionInput, actor string) (Request, error) {
	request, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Request{}, err
	}
	actor, input.Decision, input.Comment = strings.TrimSpace(actor), strings.ToLower(strings.TrimSpace(input.Decision)), strings.TrimSpace(input.Comment)
	if actor == "" || input.Revision < 1 || input.Decision != "approve" || len(input.Comment) > 2000 {
		return Request{}, ErrInvalidInput
	}
	if request.Revision != input.Revision {
		return Request{}, ErrRevisionConflict
	}
	if request.Status != StatusRollbackAwaitingApproval {
		return Request{}, ErrInvalidTransition
	}
	if actor == request.RollbackRequestedBy {
		return Request{}, ErrSelfRollbackApproval
	}
	previous, now := request.Status, s.now()
	request.Status = StatusRollbackApproved
	request.Revision++
	request.UpdatedAt = now
	request.RollbackApprovedBy = actor
	if err := s.store.Update(ctx, request, input.Revision, newEvent(request, "rollback_approved", previous, actor, input.Comment, now)); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (s *Service) TriggerRollback(ctx context.Context, id string, input TriggerInput, actor string) (Request, error) {
	if !s.Enabled() {
		return Request{}, ErrDisabled
	}
	request, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Request{}, err
	}
	if strings.TrimSpace(actor) == "" || input.Revision < 1 {
		return Request{}, ErrInvalidInput
	}
	if request.Revision != input.Revision {
		return Request{}, ErrRevisionConflict
	}
	if request.Status != StatusRollbackApproved {
		return Request{}, ErrInvalidTransition
	}
	if input.Confirmation != request.TriggerConfirmation || request.Artifact == nil {
		return Request{}, ErrConfirmation
	}
	run, err := s.connector.Rollback(ctx, RollbackRequest{ReleaseID: request.ID, Project: request.Project, Environment: request.Environment, Pipeline: request.Pipeline, RunID: request.RunID, ArtifactDigest: request.Artifact.Digest, Reason: request.RollbackReason})
	if err != nil {
		return Request{}, fmt.Errorf("%w: %v", ErrProviderRejected, err)
	}
	if !eventPattern.MatchString(run.ID) || !validHTTPSURL(run.URL) {
		return Request{}, ErrProviderRejected
	}
	previous, now := request.Status, s.now()
	request.Status = StatusRollbackQueued
	request.Revision++
	request.UpdatedAt = now
	request.RollbackRunID = run.ID
	request.RollbackRunURL = run.URL
	if err := s.store.Update(ctx, request, input.Revision, newEvent(request, "rollback_triggered", previous, actor, "controlled rollback run queued", now)); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (s *Service) Reconcile(ctx context.Context, provider ProviderEvent, payload []byte) (Request, bool, error) {
	provider.EventID, provider.ReleaseID, provider.RunID = strings.TrimSpace(provider.EventID), strings.TrimSpace(provider.ReleaseID), strings.TrimSpace(provider.RunID)
	provider.Phase, provider.Status = strings.ToLower(strings.TrimSpace(provider.Phase)), strings.ToLower(strings.TrimSpace(provider.Status))
	if !eventPattern.MatchString(provider.EventID) || !eventPattern.MatchString(provider.ReleaseID) || !eventPattern.MatchString(provider.RunID) || (provider.Phase != ProviderPhaseDeploy && provider.Phase != ProviderPhaseRollback) || !validProviderStatus(provider.Status) || len(payload) == 0 {
		return Request{}, false, ErrInvalidInput
	}
	payloadHash := sha256.Sum256(payload)
	for attempt := 0; attempt < 3; attempt++ {
		request, err := s.store.Get(ctx, provider.ReleaseID)
		if err != nil {
			return Request{}, false, err
		}
		updated, eventType, reason, err := applyProvider(request, provider, s.now())
		if err != nil {
			return Request{}, false, err
		}
		event := newEvent(updated, eventType, request.Status, "cicd-provider", reason, updated.UpdatedAt)
		event.ProviderEventID = provider.EventID
		applied, err := s.store.ApplyProviderEvent(ctx, updated, request.Revision, event, provider.EventID, hex.EncodeToString(payloadHash[:]))
		if errors.Is(err, ErrRevisionConflict) {
			continue
		}
		if err != nil {
			return Request{}, false, err
		}
		if !applied {
			current, getErr := s.store.Get(ctx, provider.ReleaseID)
			return current, false, getErr
		}
		return updated, true, nil
	}
	return Request{}, false, ErrRevisionConflict
}

func applyProvider(request Request, provider ProviderEvent, now time.Time) (Request, string, string, error) {
	expectedRun := request.RunID
	rollback := provider.Phase == ProviderPhaseRollback
	if rollback {
		expectedRun = request.RollbackRunID
	}
	if expectedRun == "" || provider.RunID != expectedRun {
		return Request{}, "", "", ErrProviderConflict
	}
	previous := request.Status
	allowed := false
	if !rollback {
		allowed = (previous == StatusQueued && (provider.Status == ProviderStatusQueued || provider.Status == ProviderStatusRunning || provider.Status == ProviderStatusSucceeded || provider.Status == ProviderStatusFailed)) || (previous == StatusRunning && (provider.Status == ProviderStatusRunning || provider.Status == ProviderStatusSucceeded || provider.Status == ProviderStatusFailed))
	} else {
		allowed = (previous == StatusRollbackQueued && (provider.Status == ProviderStatusQueued || provider.Status == ProviderStatusRunning || provider.Status == ProviderStatusSucceeded || provider.Status == ProviderStatusFailed)) || (previous == StatusRollbackRunning && (provider.Status == ProviderStatusRunning || provider.Status == ProviderStatusSucceeded || provider.Status == ProviderStatusFailed))
	}
	request.Revision++
	request.UpdatedAt = now
	if !allowed {
		return request, "provider_event_ignored", "stale or terminal provider event recorded without state regression", nil
	}
	request.Checks = normalizeChecks(provider.Checks)
	if !rollback && provider.Status == ProviderStatusSucceeded {
		if !validEvidence(provider.Artifact, request.RequiredChecks, request.Checks) || (request.PromotedArtifactDigest != "" && provider.Artifact.Digest != request.PromotedArtifactDigest) {
			request.Status = StatusFailed
			request.ErrorCode = "release_evidence_failed"
			request.ErrorMessage = ErrProviderEvidence.Error()
			request.CompletedAt = &now
			return request, "provider_evidence_failed", request.ErrorMessage, nil
		}
		artifact := *provider.Artifact
		artifact.URI = strings.TrimSpace(artifact.URI)
		artifact.Digest = strings.ToLower(strings.TrimSpace(artifact.Digest))
		artifact.SBOMURI = strings.TrimSpace(artifact.SBOMURI)
		artifact.SBOMSHA256 = strings.ToLower(strings.TrimSpace(artifact.SBOMSHA256))
		request.Artifact = &artifact
		request.Status = StatusSucceeded
		request.CompletedAt = &now
	} else if rollback && provider.Status == ProviderStatusSucceeded {
		if !checksPassed([]string{"health", "smoke"}, request.Checks) {
			request.Status = StatusRollbackFailed
			request.ErrorCode = "rollback_evidence_failed"
			request.ErrorMessage = ErrProviderEvidence.Error()
		} else {
			request.Status = StatusRolledBack
		}
		request.CompletedAt = &now
	} else if provider.Status == ProviderStatusFailed {
		if rollback {
			request.Status = StatusRollbackFailed
		} else {
			request.Status = StatusFailed
		}
		request.ErrorCode = "provider_run_failed"
		request.ErrorMessage = "CI/CD provider reported a failed run"
		request.CompletedAt = &now
	} else if provider.Status == ProviderStatusRunning {
		if rollback {
			request.Status = StatusRollbackRunning
		} else {
			request.Status = StatusRunning
		}
	} else if rollback {
		request.Status = StatusRollbackQueued
	} else {
		request.Status = StatusQueued
	}
	return request, "provider_" + provider.Phase + "_" + provider.Status, "signed provider state reconciled", nil
}

func normalizeChecks(values []CheckEvidence) []CheckEvidence {
	result := append([]CheckEvidence(nil), values...)
	for i := range result {
		result[i].Name = strings.ToLower(strings.TrimSpace(result[i].Name))
		result[i].Status = strings.ToLower(strings.TrimSpace(result[i].Status))
		result[i].EvidenceURI = strings.TrimSpace(result[i].EvidenceURI)
		result[i].SHA256 = strings.ToLower(strings.TrimSpace(result[i].SHA256))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func validEvidence(artifact *ArtifactEvidence, required []string, checks []CheckEvidence) bool {
	if artifact == nil || !artifact.SignatureVerified || !artifact.ProvenanceVerified || !digestPattern.MatchString(strings.ToLower(strings.TrimSpace(artifact.Digest))) || !hashPattern.MatchString(strings.ToLower(strings.TrimSpace(artifact.SBOMSHA256))) || !validHTTPSURL(artifact.URI) || !validHTTPSURL(artifact.SBOMURI) {
		return false
	}
	return checksPassed(required, checks)
}

func checksPassed(required []string, checks []CheckEvidence) bool {
	passed := map[string]bool{}
	for _, check := range checks {
		if check.Status == "passed" && validHTTPSURL(check.EvidenceURI) && hashPattern.MatchString(check.SHA256) {
			passed[check.Name] = true
		}
	}
	for _, name := range required {
		if !passed[name] {
			return false
		}
	}
	return true
}

func validProviderStatus(value string) bool {
	return value == ProviderStatusQueued || value == ProviderStatusRunning || value == ProviderStatusSucceeded || value == ProviderStatusFailed
}
func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}
func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func confirmation(request Request) string {
	payload, _ := json.Marshal([]string{request.ID, request.Commit, request.Environment, request.Pipeline, request.ManifestSHA256, request.ConfigurationSHA256})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func newEvent(request Request, eventType, from, actor, reason string, now time.Time) Event {
	return Event{ID: newID(), ReleaseID: request.ID, Sequence: request.Revision, Type: eventType, FromStatus: from, ToStatus: request.Status, Actor: actor, Reason: bounded(reason, 2000), CreatedAt: now}
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func (s *Service) Get(ctx context.Context, id string) (Request, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}
func (s *Service) List(ctx context.Context, project string, limit int) ([]Request, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.List(ctx, strings.ToLower(strings.TrimSpace(project)), limit)
}
func (s *Service) Events(ctx context.Context, id string) ([]Event, error) {
	return s.store.ListEvents(ctx, strings.TrimSpace(id))
}
