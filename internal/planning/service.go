package planning

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ekbda/internal/knowledge"
	"ekbda/internal/repositorysync"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

const (
	maxRequirementLength = 12000
	maxContextChanges    = 100
	maxContextRules      = 100
)

var (
	keyPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	questionPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	planIDPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)
)

type Service struct {
	store          Store
	provider       Provider
	reviewProvider RoleReviewProvider
	knowledge      *knowledge.Service
	standards      *standards.Service
	workspace      *workspace.Service
	repositorySync *repositorysync.Service
}

func NewService(store Store, provider Provider, knowledgeService *knowledge.Service, standardsService *standards.Service, workspaceService *workspace.Service, repositorySyncService *repositorysync.Service) *Service {
	reviewProvider := RoleReviewProvider(NewLocalProvider())
	if candidate, ok := provider.(RoleReviewProvider); ok {
		reviewProvider = candidate
	}
	return &Service{store: store, provider: provider, reviewProvider: reviewProvider, knowledge: knowledgeService, standards: standardsService, workspace: workspaceService, repositorySync: repositorySyncService}
}

func (s *Service) Create(ctx context.Context, input CreateInput, createdBy string, roles []string) (Session, error) {
	input, err := normalizeCreateInput(input)
	if err != nil || strings.TrimSpace(createdBy) == "" || len(createdBy) > 256 {
		return Session{}, ErrInvalidInput
	}
	contextSnapshot, err := s.gatherContext(ctx, input, roles)
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	session := Session{
		ID: newID(), Project: input.Project, Repository: contextSnapshot.Repository.Repository,
		Technology: input.Technology, Title: input.Title, Requirement: input.Requirement,
		AcceptanceCriteria: input.AcceptanceCriteria, Constraints: input.Constraints,
		OutOfScope: input.OutOfScope, Status: StatusAwaitingClarification, Revision: 1,
		Answers: []ClarificationAnswer{}, Context: contextSnapshot, Provider: s.provider.Name(),
		CreatedBy: strings.TrimSpace(createdBy), CreatedAt: now, UpdatedAt: now,
	}
	session.Questions, err = s.provider.Clarify(ctx, session)
	if err != nil {
		return Session{}, fmt.Errorf("%w: generate planning clarifications: %v", ErrProviderFailed, err)
	}
	if err := validateQuestions(session.Questions); err != nil {
		return Session{}, err
	}
	if len(session.Questions) == 0 {
		plan, err := s.provider.BuildPlan(ctx, session)
		if err != nil {
			return Session{}, fmt.Errorf("%w: generate implementation plan: %v", ErrProviderFailed, err)
		}
		if err := validatePlan(plan, session.Context); err != nil {
			return Session{}, err
		}
		session.Plan = &plan
		session.Status = StatusAwaitingRoleReview
	}
	session.Context = safeContext(session.Context)
	event := Event{ID: newID(), SessionID: session.ID, Sequence: 1, Type: "created", ToStatus: session.Status, Actor: session.CreatedBy, CreatedAt: now}
	if err := s.store.Create(ctx, session, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) SubmitClarifications(ctx context.Context, id string, input ClarificationInput, actor string, roles []string, governanceOverride bool) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	actor = strings.TrimSpace(actor)
	if session.Status != StatusAwaitingClarification {
		return Session{}, ErrInvalidTransition
	}
	if actor != session.CreatedBy && !governanceOverride {
		return Session{}, ErrForbiddenParticipant
	}
	if input.Revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	answers, err := validateAnswers(session.Questions, input.Answers)
	if err != nil {
		return Session{}, err
	}
	contextSnapshot, err := s.gatherContext(ctx, CreateInput{
		Project: session.Project, Repository: session.Repository, Technology: session.Technology,
		Title: session.Title, Requirement: session.Requirement,
		AcceptanceCriteria: session.AcceptanceCriteria, Constraints: session.Constraints, OutOfScope: session.OutOfScope,
	}, roles)
	if err != nil {
		return Session{}, err
	}
	session.Answers = answers
	session.Context = contextSnapshot
	plan, err := s.provider.BuildPlan(ctx, session)
	if err != nil {
		return Session{}, fmt.Errorf("%w: generate implementation plan: %v", ErrProviderFailed, err)
	}
	if err := validatePlan(plan, session.Context); err != nil {
		return Session{}, err
	}
	previousStatus := session.Status
	session.Plan = &plan
	session.Status = StatusAwaitingRoleReview
	session.Revision++
	session.Provider = s.provider.Name()
	session.Context = safeContext(session.Context)
	session.UpdatedAt = time.Now().UTC()
	event := Event{
		ID: newID(), SessionID: session.ID, Sequence: session.Revision,
		Type: "clarifications_submitted", FromStatus: previousStatus, ToStatus: session.Status,
		Actor: actor, Reason: fmt.Sprintf("answered %d clarification questions", len(answers)), CreatedAt: session.UpdatedAt,
	}
	if err := s.store.Update(ctx, session, input.Revision, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) SubmitRoleReviews(ctx context.Context, id string, revision int, actor string, roles []string, governanceOverride bool) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	actor = strings.TrimSpace(actor)
	if session.Status != StatusAwaitingRoleReview || session.Plan == nil {
		return Session{}, ErrInvalidTransition
	}
	if actor != session.CreatedBy && !governanceOverride {
		return Session{}, ErrForbiddenParticipant
	}
	if revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	reviewContext, err := s.gatherContext(ctx, CreateInput{
		Project: session.Project, Repository: session.Repository, Technology: session.Technology,
		Title: session.Title, Requirement: session.Requirement,
		AcceptanceCriteria: session.AcceptanceCriteria, Constraints: session.Constraints, OutOfScope: session.OutOfScope,
	}, roles)
	if err != nil {
		return Session{}, err
	}
	session.RoleReview = &RoleReviewCycle{Provider: s.reviewProvider.ReviewName(), Context: reviewContext}
	reviews := make([]RoleReview, len(requiredReviewRoles))
	reviewContextWithCancel, cancel := context.WithCancel(ctx)
	defer cancel()
	var waitGroup sync.WaitGroup
	errorsChannel := make(chan error, len(requiredReviewRoles))
	for index, role := range requiredReviewRoles {
		waitGroup.Add(1)
		go func(index int, role string) {
			defer waitGroup.Done()
			review, reviewErr := s.reviewProvider.ReviewRole(reviewContextWithCancel, role, session)
			if reviewErr != nil {
				errorsChannel <- reviewErr
				cancel()
				return
			}
			review.Role = role
			reviews[index] = review
		}(index, role)
	}
	waitGroup.Wait()
	close(errorsChannel)
	if reviewErr, found := <-errorsChannel; found {
		return Session{}, fmt.Errorf("%w: generate role reviews: %v", ErrProviderFailed, reviewErr)
	}
	for _, review := range reviews {
		if err := validateRoleReview(review, reviewContext); err != nil {
			return Session{}, err
		}
	}
	coordination, err := s.reviewProvider.Coordinate(ctx, session, reviews)
	if err != nil {
		return Session{}, fmt.Errorf("%w: coordinate role reviews: %v", ErrProviderFailed, err)
	}
	if err := validateCoordination(coordination); err != nil {
		return Session{}, err
	}
	previousStatus := session.Status
	session.RoleReview.Reviews = reviews
	session.RoleReview.Coordination = coordination
	session.RoleReview.Context = safeContext(reviewContext)
	now := time.Now().UTC()
	session.RoleReview.CompletedAt = now
	if len(coordination.DecisionItems) == 0 {
		session.Status = StatusAwaitingApproval
	} else {
		session.Status = StatusAwaitingResolution
	}
	session.Revision++
	session.UpdatedAt = now
	event := Event{
		ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: "role_reviews_completed",
		FromStatus: previousStatus, ToStatus: session.Status, Actor: actor,
		Reason: fmt.Sprintf("completed %d role reviews with %d decision items", len(reviews), len(coordination.DecisionItems)), CreatedAt: now,
	}
	if err := s.store.Update(ctx, session, revision, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) ResolveReviewDecisions(ctx context.Context, id string, input ResolutionInput, actor string) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	actor = strings.TrimSpace(actor)
	if session.Status != StatusAwaitingResolution || session.RoleReview == nil {
		return Session{}, ErrInvalidTransition
	}
	if input.Revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	if actor == "" || actor == session.CreatedBy {
		return Session{}, ErrSelfResolution
	}
	resolutions, err := validateResolutions(session.RoleReview.Coordination.DecisionItems, input.Resolutions)
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	for index := range session.RoleReview.Coordination.DecisionItems {
		decision := &session.RoleReview.Coordination.DecisionItems[index]
		decision.Resolution = resolutions[decision.ID]
		decision.ResolvedBy = actor
		decision.ResolvedAt = &now
	}
	previousStatus := session.Status
	session.Status = StatusAwaitingApproval
	session.Revision++
	session.UpdatedAt = now
	event := Event{
		ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: "review_decisions_resolved",
		FromStatus: previousStatus, ToStatus: session.Status, Actor: actor,
		Reason: fmt.Sprintf("resolved %d role review decisions", len(resolutions)), CreatedAt: now,
	}
	if err := s.store.Update(ctx, session, input.Revision, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Decide(ctx context.Context, id string, input DecisionInput, reviewer string) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	reviewer = strings.TrimSpace(reviewer)
	if session.Status != StatusAwaitingApproval || session.Plan == nil {
		return Session{}, ErrInvalidTransition
	}
	if input.Revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	if reviewer == "" || reviewer == session.CreatedBy {
		return Session{}, ErrSelfApproval
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	reason := strings.TrimSpace(input.Reason)
	if (decision != "approve" && decision != "reject") || len(reason) > 2000 || (decision == "reject" && reason == "") {
		return Session{}, ErrInvalidInput
	}
	previousStatus := session.Status
	if decision == "approve" {
		session.Status = StatusApproved
	} else {
		session.Status = StatusRejected
	}
	now := time.Now().UTC()
	session.Revision++
	session.ReviewedBy = reviewer
	session.DecisionReason = reason
	session.ReviewedAt = &now
	session.UpdatedAt = now
	event := Event{
		ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: decision + "d",
		FromStatus: previousStatus, ToStatus: session.Status, Actor: reviewer, Reason: reason, CreatedAt: now,
	}
	if err := s.store.Update(ctx, session, input.Revision, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Get(ctx context.Context, id string) (Session, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, project string, limit int) ([]Session, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.List(ctx, strings.ToLower(strings.TrimSpace(project)), limit)
}

func (s *Service) Events(ctx context.Context, id string) ([]Event, error) {
	return s.store.ListEvents(ctx, strings.TrimSpace(id))
}

func (s *Service) gatherContext(ctx context.Context, input CreateInput, roles []string) (ContextSnapshot, error) {
	state, err := s.workspace.Inspect(ctx, input.Repository)
	if err != nil {
		return ContextSnapshot{}, err
	}
	results, err := s.knowledge.Search(ctx, knowledge.SearchInput{Query: input.Requirement, Project: input.Project, Roles: roles, Limit: 8})
	if err != nil {
		return ContextSnapshot{}, fmt.Errorf("retrieve planning knowledge: %w", err)
	}
	packages, err := s.standards.ApplicablePackages(ctx, input.Project, input.Technology)
	if err != nil {
		return ContextSnapshot{}, fmt.Errorf("resolve planning standards: %w", err)
	}
	snapshot := ContextSnapshot{
		Knowledge: make([]KnowledgeReference, 0, len(results)), Standards: make([]StandardReference, 0, len(packages)),
		Repository: RepositoryContext{
			Repository: state.Repository, HeadCommit: state.HeadCommit, Branch: state.Branch,
			Dirty: state.Dirty, TrackedCount: state.TrackedCount, UntrackedCount: state.UntrackedCount,
			ChangedCount: len(state.Changes), ChangedPaths: make([]string, 0),
		},
		CapturedAt: time.Now().UTC(),
	}
	for index, result := range results {
		digest := sha256.Sum256([]byte(result.Snippet))
		citation := result.Citation
		snapshot.Knowledge = append(snapshot.Knowledge, KnowledgeReference{
			ID: fmt.Sprintf("K%d", index+1), DocumentID: citation.DocumentID,
			Version: citation.Version, ChunkIndex: citation.ChunkIndex, SnippetHash: hex.EncodeToString(digest[:]),
			Title: citation.Title, Snippet: result.Snippet, Citation: &citation,
		})
	}
	ruleCount := 0
	for index, standard := range packages {
		reference := StandardReference{
			ID: fmt.Sprintf("S%d", index+1), PackageID: standard.ID, Name: standard.Name,
			Scope: standard.Scope, Selector: standard.Selector, Version: standard.Version,
			DefinitionHash: standard.DefinitionHash, Rules: make([]RuleReference, 0),
		}
		for _, rule := range standard.Rules {
			if ruleCount >= maxContextRules {
				break
			}
			reference.Rules = append(reference.Rules, RuleReference{
				ID: rule.ID, Title: rule.Title, Description: truncate(rule.Description, 500),
				Category: rule.Category, Level: rule.Level,
			})
			ruleCount++
		}
		snapshot.Standards = append(snapshot.Standards, reference)
	}
	for index, change := range state.Changes {
		if index >= maxContextChanges {
			snapshot.Repository.ChangesTruncated = true
			break
		}
		snapshot.Repository.ChangedPaths = append(snapshot.Repository.ChangedPaths, change.Path)
	}
	if lastSync, err := s.repositorySync.LatestCompleted(ctx, input.Project, state.Repository); err == nil {
		snapshot.Repository.LastSyncID = lastSync.ID
		snapshot.Repository.LastSyncCommit = lastSync.HeadCommit
	} else if !errors.Is(err, repositorysync.ErrReportNotFound) {
		return ContextSnapshot{}, fmt.Errorf("load repository knowledge sync baseline: %w", err)
	}
	safe := safeContext(snapshot)
	encoded, err := json.Marshal(safe)
	if err != nil {
		return ContextSnapshot{}, fmt.Errorf("encode planning context: %w", err)
	}
	digest := sha256.Sum256(encoded)
	snapshot.Hash = hex.EncodeToString(digest[:])
	return snapshot, nil
}

func safeContext(snapshot ContextSnapshot) ContextSnapshot {
	result := snapshot
	result.Knowledge = append([]KnowledgeReference(nil), snapshot.Knowledge...)
	for index := range result.Knowledge {
		result.Knowledge[index].Title = ""
		result.Knowledge[index].Snippet = ""
		result.Knowledge[index].Citation = nil
	}
	return result
}

func normalizeCreateInput(input CreateInput) (CreateInput, error) {
	input.Project = strings.ToLower(strings.TrimSpace(input.Project))
	input.Technology = strings.ToLower(strings.TrimSpace(input.Technology))
	input.Repository = strings.TrimSpace(strings.ReplaceAll(input.Repository, "\\", "/"))
	if input.Repository != "" {
		input.Repository = path.Clean(input.Repository)
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Requirement = strings.TrimSpace(input.Requirement)
	var err error
	input.AcceptanceCriteria, err = normalizeTextList(input.AcceptanceCriteria)
	if err != nil {
		return CreateInput{}, err
	}
	input.Constraints, err = normalizeTextList(input.Constraints)
	if err != nil {
		return CreateInput{}, err
	}
	input.OutOfScope, err = normalizeTextList(input.OutOfScope)
	if err != nil {
		return CreateInput{}, err
	}
	if !keyPattern.MatchString(input.Project) || !keyPattern.MatchString(input.Technology) || input.Repository == "" || input.Repository == ".." || strings.HasPrefix(input.Repository, "../") ||
		input.Title == "" || len(input.Title) > 200 || len(input.Requirement) < 20 || len(input.Requirement) > maxRequirementLength || !utf8.ValidString(input.Requirement) {
		return CreateInput{}, ErrInvalidInput
	}
	return input, nil
}

func normalizeTextList(values []string) ([]string, error) {
	if len(values) > 20 {
		return nil, ErrInvalidInput
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 500 || !utf8.ValidString(value) {
			return nil, ErrInvalidInput
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validateQuestions(questions []Question) error {
	if len(questions) > 3 {
		return ErrInvalidProviderOutput
	}
	seen := make(map[string]struct{}, len(questions))
	for index := range questions {
		questions[index].ID = strings.ToLower(strings.TrimSpace(questions[index].ID))
		questions[index].Question = strings.TrimSpace(questions[index].Question)
		questions[index].Reason = strings.TrimSpace(questions[index].Reason)
		if !questionPattern.MatchString(questions[index].ID) || questions[index].Question == "" || len(questions[index].Question) > 1000 || len(questions[index].Reason) > 1000 {
			return ErrInvalidProviderOutput
		}
		if _, found := seen[questions[index].ID]; found {
			return ErrInvalidProviderOutput
		}
		seen[questions[index].ID] = struct{}{}
	}
	return nil
}

func validateAnswers(questions []Question, answers []ClarificationAnswer) ([]ClarificationAnswer, error) {
	if len(answers) != len(questions) {
		return nil, ErrIncompleteAnswers
	}
	wanted := make(map[string]struct{}, len(questions))
	for _, question := range questions {
		wanted[question.ID] = struct{}{}
	}
	result := append([]ClarificationAnswer(nil), answers...)
	seen := make(map[string]struct{}, len(answers))
	for index := range result {
		result[index].QuestionID = strings.ToLower(strings.TrimSpace(result[index].QuestionID))
		result[index].Answer = strings.TrimSpace(result[index].Answer)
		_, valid := wanted[result[index].QuestionID]
		_, duplicate := seen[result[index].QuestionID]
		if !valid || duplicate || result[index].Answer == "" || len(result[index].Answer) > 4000 || !utf8.ValidString(result[index].Answer) {
			return nil, ErrIncompleteAnswers
		}
		seen[result[index].QuestionID] = struct{}{}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].QuestionID < result[j].QuestionID })
	return result, nil
}

func validatePlan(plan Plan, snapshot ContextSnapshot) error {
	plan.Summary = strings.TrimSpace(plan.Summary)
	if plan.Summary == "" || len(plan.Summary) > 4000 || len(plan.Steps) < 1 || len(plan.Steps) > 20 || len(plan.Assumptions) > 20 || len(plan.Risks) > 20 || len(plan.OutOfScope) > 20 {
		return ErrInvalidProviderOutput
	}
	knowledgeIDs := make(map[string]struct{}, len(snapshot.Knowledge))
	for _, reference := range snapshot.Knowledge {
		knowledgeIDs[reference.ID] = struct{}{}
	}
	standardIDs := make(map[string]struct{}, len(snapshot.Standards))
	for _, reference := range snapshot.Standards {
		standardIDs[reference.ID] = struct{}{}
	}
	stepIDs := make(map[string]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		if !planIDPattern.MatchString(step.ID) || strings.TrimSpace(step.Title) == "" || len(step.Title) > 200 || strings.TrimSpace(step.Description) == "" || len(step.Description) > 4000 || len(step.Deliverables) < 1 || len(step.Deliverables) > 20 || len(step.Verification) < 1 || len(step.Verification) > 20 {
			return ErrInvalidProviderOutput
		}
		if _, found := stepIDs[step.ID]; found {
			return ErrInvalidProviderOutput
		}
		stepIDs[step.ID] = struct{}{}
		if !validShortList(step.Deliverables, 500) || !validShortList(step.Verification, 500) || !referencesAllowed(step.KnowledgeReferences, knowledgeIDs) || !referencesAllowed(step.StandardReferences, standardIDs) {
			return ErrInvalidProviderOutput
		}
	}
	riskIDs := make(map[string]struct{}, len(plan.Risks))
	for _, risk := range plan.Risks {
		if !planIDPattern.MatchString(risk.ID) || strings.TrimSpace(risk.Description) == "" || len(risk.Description) > 2000 || strings.TrimSpace(risk.Mitigation) == "" || len(risk.Mitigation) > 2000 {
			return ErrInvalidProviderOutput
		}
		if _, found := riskIDs[risk.ID]; found {
			return ErrInvalidProviderOutput
		}
		riskIDs[risk.ID] = struct{}{}
	}
	if !validShortList(plan.Assumptions, 1000) || !validShortList(plan.OutOfScope, 1000) {
		return ErrInvalidProviderOutput
	}
	return nil
}

func validateRoleReview(review RoleReview, snapshot ContextSnapshot) error {
	if !reviewRoleAllowed(review.Role) || strings.TrimSpace(review.Summary) == "" || len(review.Summary) > 4000 ||
		(review.Recommendation != "approve" && review.Recommendation != "approve_with_conditions" && review.Recommendation != "reject") ||
		len(review.Findings) > 20 || len(review.OpenQuestions) > 10 {
		return ErrInvalidProviderOutput
	}
	knowledgeIDs := make(map[string]struct{}, len(snapshot.Knowledge))
	for _, reference := range snapshot.Knowledge {
		knowledgeIDs[reference.ID] = struct{}{}
	}
	standardIDs := make(map[string]struct{}, len(snapshot.Standards))
	for _, reference := range snapshot.Standards {
		standardIDs[reference.ID] = struct{}{}
	}
	if !referencesAllowed(review.KnowledgeReferences, knowledgeIDs) || !referencesAllowed(review.StandardReferences, standardIDs) || !validShortList(review.OpenQuestions, 1000) {
		return ErrInvalidProviderOutput
	}
	findingIDs := make(map[string]struct{}, len(review.Findings))
	for _, finding := range review.Findings {
		if !planIDPattern.MatchString(finding.ID) || (finding.Severity != "info" && finding.Severity != "warning" && finding.Severity != "blocking") ||
			strings.TrimSpace(finding.Statement) == "" || len(finding.Statement) > 2000 || strings.TrimSpace(finding.Recommendation) == "" || len(finding.Recommendation) > 2000 {
			return ErrInvalidProviderOutput
		}
		if _, found := findingIDs[finding.ID]; found {
			return ErrInvalidProviderOutput
		}
		findingIDs[finding.ID] = struct{}{}
	}
	return nil
}

func validateCoordination(coordination Coordination) error {
	if strings.TrimSpace(coordination.Summary) == "" || len(coordination.Summary) > 4000 || len(coordination.Consensus) > 20 || len(coordination.DecisionItems) > 20 || !validShortList(coordination.Consensus, 1000) {
		return ErrInvalidProviderOutput
	}
	decisionIDs := make(map[string]struct{}, len(coordination.DecisionItems))
	for _, decision := range coordination.DecisionItems {
		if !planIDPattern.MatchString(decision.ID) || strings.TrimSpace(decision.Topic) == "" || len(decision.Topic) > 500 ||
			strings.TrimSpace(decision.Description) == "" || len(decision.Description) > 2000 || len(decision.Options) < 2 || len(decision.Options) > 5 ||
			!validShortList(decision.Options, 1000) || len(decision.SourceRoles) == 0 || len(decision.SourceRoles) > len(requiredReviewRoles) ||
			decision.Resolution != "" || decision.ResolvedBy != "" || decision.ResolvedAt != nil {
			return ErrInvalidProviderOutput
		}
		if _, found := decisionIDs[decision.ID]; found {
			return ErrInvalidProviderOutput
		}
		decisionIDs[decision.ID] = struct{}{}
		seenRoles := make(map[string]struct{}, len(decision.SourceRoles))
		for _, role := range decision.SourceRoles {
			if !reviewRoleAllowed(role) {
				return ErrInvalidProviderOutput
			}
			if _, found := seenRoles[role]; found {
				return ErrInvalidProviderOutput
			}
			seenRoles[role] = struct{}{}
		}
	}
	return nil
}

func validateResolutions(decisions []DecisionItem, input []Resolution) (map[string]string, error) {
	if len(input) != len(decisions) || len(decisions) == 0 {
		return nil, ErrIncompleteResolutions
	}
	wanted := make(map[string]struct{}, len(decisions))
	for _, decision := range decisions {
		wanted[decision.ID] = struct{}{}
	}
	result := make(map[string]string, len(input))
	for _, resolution := range input {
		resolution.DecisionID = strings.TrimSpace(resolution.DecisionID)
		resolution.Resolution = strings.TrimSpace(resolution.Resolution)
		_, valid := wanted[resolution.DecisionID]
		_, duplicate := result[resolution.DecisionID]
		if !valid || duplicate || resolution.Resolution == "" || len(resolution.Resolution) > 2000 || !utf8.ValidString(resolution.Resolution) {
			return nil, ErrIncompleteResolutions
		}
		result[resolution.DecisionID] = resolution.Resolution
	}
	return result, nil
}

func reviewRoleAllowed(role string) bool {
	for _, expected := range requiredReviewRoles {
		if role == expected {
			return true
		}
	}
	return false
}

func referencesAllowed(references []string, allowed map[string]struct{}) bool {
	if len(references) > 20 {
		return false
	}
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if _, found := allowed[reference]; !found {
			return false
		}
		if _, duplicate := seen[reference]; duplicate {
			return false
		}
		seen[reference] = struct{}{}
	}
	return true
}

func validShortList(values []string, maxLength int) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > maxLength || !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("planning-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
