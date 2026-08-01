package development

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ekbda/internal/initiative"
	"ekbda/internal/workspace"
)

var developmentKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type Service struct {
	store      Store
	initiative initiativeReader
	workspace  workspaceInspector
	runner     Runner
	deliverer  Deliverer
}

type initiativeReader interface {
	Get(context.Context, string) (initiative.Package, error)
	Reviews(context.Context, string, string, int) ([]initiative.ArtifactReview, error)
}

type workspaceInspector interface {
	Inspect(context.Context, string) (workspace.RepositoryState, error)
}

func NewService(store Store, initiativeService initiativeReader, workspaceService workspaceInspector) *Service {
	return NewServiceWithRunner(store, initiativeService, workspaceService, disabledRunner{})
}

func NewServiceWithRunner(store Store, initiativeService initiativeReader, workspaceService workspaceInspector, runner Runner) *Service {
	return NewServiceWithDelivery(store, initiativeService, workspaceService, runner, disabledDeliverer{})
}

func NewServiceWithDelivery(store Store, initiativeService initiativeReader, workspaceService workspaceInspector, runner Runner, deliverer Deliverer) *Service {
	if runner == nil {
		runner = disabledRunner{}
	}
	if deliverer == nil {
		deliverer = disabledDeliverer{}
	}
	return &Service{store: store, initiative: initiativeService, workspace: workspaceService, runner: runner, deliverer: deliverer}
}

func (s *Service) ExecutionEnabled() bool { return s != nil && s.runner.Enabled() }

func (s *Service) DeliveryEnabled() bool { return s != nil && s.deliverer.Enabled() }

func (s *Service) Create(ctx context.Context, input CreateInput, actor string) (Session, error) {
	input.ProjectPackageID = strings.TrimSpace(input.ProjectPackageID)
	actor = strings.TrimSpace(actor)
	allowedPaths, err := normalizeAllowedPaths(input.AllowedPaths)
	if err != nil || input.ProjectPackageID == "" || actor == "" || len(actor) > 256 {
		return Session{}, ErrInvalidInput
	}
	allowedCommands, err := validateAllowedCommands(input.AllowedCommands)
	if err != nil {
		return Session{}, err
	}
	projectPackage, err := s.initiative.Get(ctx, input.ProjectPackageID)
	if err != nil {
		return Session{}, err
	}
	if err := s.requireApprovedPackage(ctx, projectPackage); err != nil {
		return Session{}, err
	}
	state, err := s.workspace.Inspect(ctx, projectPackage.Repository)
	if err != nil {
		return Session{}, err
	}
	if state.Dirty {
		return Session{}, ErrDirtyWorkspace
	}
	if state.HeadCommit == "" {
		return Session{}, ErrMissingBaseline
	}
	now := time.Now().UTC()
	technology := strings.ToLower(strings.TrimSpace(input.Technology))
	if technology == "" {
		technology = "go"
	}
	if !developmentKeyPattern.MatchString(technology) {
		return Session{}, ErrInvalidInput
	}
	id := newID()
	session := Session{
		ID: id, Project: projectPackage.Project, Repository: projectPackage.Repository,
		ProjectPackageID: projectPackage.ID, ProjectPackageName: projectPackage.Name, Technology: technology,
		PackageVersion: projectPackage.Version, PackageHash: projectPackage.DefinitionHash,
		PlanningSessionID: projectPackage.Source.PlanningSessionID,
		BaselineCommit:    state.HeadCommit, BaselineBranch: state.Branch,
		PlannedBranch: "codex/" + projectPackage.Project + "/" + id[:12],
		AllowedPaths:  allowedPaths, AllowedCommands: allowedCommands,
		Status: StatusDraft, Revision: 1, CreatedBy: actor, CreatedAt: now, UpdatedAt: now,
	}
	event := Event{ID: newID(), SessionID: id, Sequence: 1, Type: EventCreated, ToStatus: StatusDraft, Actor: actor, CreatedAt: now}
	if err := s.store.Create(ctx, session, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Execute(ctx context.Context, id string, input ExecuteInput, actor string) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	actor = strings.TrimSpace(actor)
	input.PatchHash = strings.TrimSpace(input.PatchHash)
	input.Confirmation = strings.TrimSpace(input.Confirmation)
	if actor == "" || actor != session.CreatedBy {
		return Session{}, ErrForbiddenActor
	}
	if input.Revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	if session.Status != StatusApproved || session.Proposal == nil {
		return Session{}, ErrInvalidTransition
	}
	if input.Confirmation != "execute_approved_patch" || input.PatchHash == "" || input.PatchHash != session.Proposal.PatchHash {
		return Session{}, ErrExecutionConflict
	}
	if !s.runner.Enabled() {
		return Session{}, ErrExecutionDisabled
	}
	if err := s.validateBaseline(ctx, session); err != nil {
		return Session{}, err
	}
	projectPackage, err := s.initiative.Get(ctx, session.ProjectPackageID)
	if err != nil {
		return Session{}, err
	}
	if projectPackage.DefinitionHash != session.PackageHash {
		return Session{}, ErrPackageNotApproved
	}
	if err := s.requireApprovedPackage(ctx, projectPackage); err != nil {
		return Session{}, err
	}
	commandIDs := make([]string, 0, len(session.Proposal.Commands))
	for _, command := range session.Proposal.Commands {
		commandIDs = append(commandIDs, command.ID)
	}
	if patchHash(session.BaselineCommit, session.Proposal.Patch, commandIDs) != session.Proposal.PatchHash {
		return Session{}, ErrExecutionConflict
	}
	now := time.Now().UTC()
	executionID := newID()
	isolation, networkPolicy := "configured_execution_runner", "configured_execution_policy"
	if profiled, ok := s.runner.(interface{ Profile() (string, string) }); ok {
		isolation, networkPolicy = profiled.Profile()
	}
	expectedRevision := session.Revision
	session.Status = StatusExecuting
	session.Revision++
	session.UpdatedAt = now
	session.Execution = &Execution{
		ID: executionID, Status: ExecutionRunning, BaselineCommit: session.BaselineCommit,
		PatchHash: session.Proposal.PatchHash, Isolation: isolation,
		NetworkPolicy: networkPolicy, ExecutedBy: actor, StartedAt: now,
	}
	startEvent := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: EventExecutionStarted, FromStatus: StatusApproved, ToStatus: StatusExecuting, Actor: actor, Reason: executionID, CreatedAt: now}
	if err := s.store.Update(ctx, session, expectedRevision, startEvent); err != nil {
		return Session{}, err
	}
	execution := s.runner.Run(ctx, RunRequest{
		ExecutionID: executionID, SessionID: session.ID, Repository: session.Repository,
		Project: session.Project, Technology: session.Technology, BaselineCommit: session.BaselineCommit,
		PatchHash: session.Proposal.PatchHash, Patch: session.Proposal.Patch,
		Files: append([]FileChange(nil), session.Proposal.Files...), Commands: cloneCommands(session.Proposal.Commands), Actor: actor,
	})
	finished := time.Now().UTC()
	expectedRevision = session.Revision
	session.Execution = &execution
	session.UpdatedAt = finished
	session.Revision++
	eventType := EventExecutionFailed
	session.Status = StatusExecutionFailed
	if execution.Status == ExecutionPassed {
		eventType = EventExecutionPassed
		session.Status = StatusVerified
	}
	reason := execution.ErrorCode
	if reason == "" {
		reason = execution.ID
	}
	finishEvent := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: eventType, FromStatus: StatusExecuting, ToStatus: session.Status, Actor: actor, Reason: reason, CreatedAt: finished}
	if err := s.store.Update(ctx, session, expectedRevision, finishEvent); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Deliver(ctx context.Context, id string, input DeliverInput, actor string) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	actor = strings.TrimSpace(actor)
	input.PatchHash = strings.TrimSpace(input.PatchHash)
	input.Confirmation = strings.TrimSpace(input.Confirmation)
	if actor == "" {
		return Session{}, ErrForbiddenActor
	}
	if actor == session.CreatedBy {
		return Session{}, ErrSelfDelivery
	}
	if input.Revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	if session.Status != StatusVerified || session.Proposal == nil || session.Execution == nil || session.Execution.Status != ExecutionPassed {
		return Session{}, ErrInvalidTransition
	}
	if input.Confirmation != "deliver_verified_change" || input.PatchHash == "" || input.PatchHash != session.Proposal.PatchHash {
		return Session{}, ErrDeliveryConflict
	}
	if !s.deliverer.Enabled() {
		return Session{}, ErrDeliveryDisabled
	}
	if err := s.validateBaseline(ctx, session); err != nil {
		return Session{}, err
	}
	projectPackage, err := s.initiative.Get(ctx, session.ProjectPackageID)
	if err != nil {
		return Session{}, err
	}
	if projectPackage.DefinitionHash != session.PackageHash {
		return Session{}, ErrPackageNotApproved
	}
	if err := s.requireApprovedPackage(ctx, projectPackage); err != nil {
		return Session{}, err
	}
	commandIDs := make([]string, 0, len(session.Proposal.Commands))
	for _, command := range session.Proposal.Commands {
		commandIDs = append(commandIDs, command.ID)
	}
	if patchHash(session.BaselineCommit, session.Proposal.Patch, commandIDs) != session.Proposal.PatchHash {
		return Session{}, ErrDeliveryConflict
	}
	now := time.Now().UTC()
	deliveryID := newID()
	expectedRevision := session.Revision
	session.Status = StatusDelivering
	session.Revision++
	session.UpdatedAt = now
	session.Delivery = &Delivery{
		ID: deliveryID, Status: DeliveryRunning, Branch: session.PlannedBranch,
		DeliveredBy: actor, StartedAt: now,
	}
	startEvent := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: EventDeliveryStarted, FromStatus: StatusVerified, ToStatus: StatusDelivering, Actor: actor, Reason: deliveryID, CreatedAt: now}
	if err := s.store.Update(ctx, session, expectedRevision, startEvent); err != nil {
		return Session{}, err
	}
	delivery := s.deliverer.Deliver(ctx, DeliveryRequest{
		DeliveryID: deliveryID, SessionID: session.ID, Repository: session.Repository,
		Project: session.Project, BaselineCommit: session.BaselineCommit, BaselineBranch: session.BaselineBranch,
		Branch: session.PlannedBranch, PatchHash: session.Proposal.PatchHash, Patch: session.Proposal.Patch,
		Files: append([]FileChange(nil), session.Proposal.Files...), Summary: session.Proposal.Summary, Actor: actor,
	})
	finished := time.Now().UTC()
	expectedRevision = session.Revision
	session.Delivery = &delivery
	session.UpdatedAt = finished
	session.Revision++
	eventType := EventDeliveryFailed
	session.Status = StatusDeliveryFailed
	if delivery.Status == DeliveryPassed {
		eventType = EventDeliveryPassed
		session.Status = StatusDelivered
	}
	reason := delivery.ErrorCode
	if reason == "" {
		reason = delivery.PullRequestURL
	}
	finishEvent := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: eventType, FromStatus: StatusDelivering, ToStatus: session.Status, Actor: actor, Reason: reason, CreatedAt: finished}
	if err := s.store.Update(ctx, session, expectedRevision, finishEvent); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Recover(ctx context.Context) error {
	if s.runner.Enabled() {
		if err := s.recoverExecutions(ctx); err != nil {
			return err
		}
	}
	if s.deliverer.Enabled() {
		return s.recoverDeliveries(ctx)
	}
	return nil
}

func (s *Service) recoverExecutions(ctx context.Context) error {
	sessions, err := s.store.ListExecuting(ctx, 1000)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	cutoff := now.Add(-s.runner.RecoveryGracePeriod())
	cleanupIDs := make([]string, 0)
	for _, session := range sessions {
		if session.UpdatedAt.After(cutoff) {
			continue
		}
		expectedRevision := session.Revision
		interruptedAt := session.UpdatedAt
		session.Status = StatusExecutionFailed
		session.Revision++
		session.UpdatedAt = now
		if session.Execution == nil {
			session.Execution = &Execution{ID: newID(), StartedAt: interruptedAt}
		}
		session.Execution.Status = ExecutionFailed
		session.Execution.ErrorCode = "execution_interrupted"
		session.Execution.ErrorMessage = "execution was interrupted before evidence was persisted"
		session.Execution.FinishedAt = now
		session.Execution.DurationMS = now.Sub(session.Execution.StartedAt).Milliseconds()
		event := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: EventExecutionRecovered, FromStatus: StatusExecuting, ToStatus: StatusExecutionFailed, Actor: "system", Reason: "execution_interrupted", CreatedAt: now}
		if err := s.store.Update(ctx, session, expectedRevision, event); err != nil {
			if errors.Is(err, ErrRevisionConflict) {
				continue
			}
			return err
		}
		if session.Execution != nil {
			cleanupIDs = append(cleanupIDs, session.Execution.ID)
		}
	}
	return s.runner.CleanupStale(ctx, cleanupIDs)
}

func (s *Service) recoverDeliveries(ctx context.Context) error {
	sessions, err := s.store.ListDelivering(ctx, 1000)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	cutoff := now.Add(-s.deliverer.RecoveryGracePeriod())
	cleanupIDs := make([]string, 0)
	for _, session := range sessions {
		if session.UpdatedAt.After(cutoff) {
			continue
		}
		expectedRevision := session.Revision
		interruptedAt := session.UpdatedAt
		session.Status = StatusDeliveryFailed
		session.Revision++
		session.UpdatedAt = now
		if session.Delivery == nil {
			session.Delivery = &Delivery{ID: newID(), Branch: session.PlannedBranch, StartedAt: interruptedAt}
		}
		session.Delivery.Status = DeliveryFailed
		session.Delivery.ErrorCode = "delivery_interrupted"
		session.Delivery.ErrorMessage = "delivery was interrupted before evidence was persisted; remote reconciliation is required"
		session.Delivery.FinishedAt = now
		session.Delivery.DurationMS = now.Sub(session.Delivery.StartedAt).Milliseconds()
		event := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: EventDeliveryRecovered, FromStatus: StatusDelivering, ToStatus: StatusDeliveryFailed, Actor: "system", Reason: "delivery_interrupted", CreatedAt: now}
		if err := s.store.Update(ctx, session, expectedRevision, event); err != nil {
			if errors.Is(err, ErrRevisionConflict) {
				continue
			}
			return err
		}
		cleanupIDs = append(cleanupIDs, session.Delivery.ID)
	}
	return s.deliverer.CleanupStale(ctx, cleanupIDs)
}

func (s *Service) Submit(ctx context.Context, id string, input SubmitInput, actor string) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	actor = strings.TrimSpace(actor)
	input.Summary = strings.TrimSpace(input.Summary)
	if actor == "" || actor != session.CreatedBy {
		return Session{}, ErrForbiddenActor
	}
	if input.Revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	if session.Status != StatusDraft || input.Summary == "" || len(input.Summary) > 2000 || !utf8.ValidString(input.Summary) || len(input.CommandIDs) == 0 {
		return Session{}, ErrInvalidTransition
	}
	if err := s.validateBaseline(ctx, session); err != nil {
		return Session{}, err
	}
	commands, commandIDs, err := resolveCommands(input.CommandIDs, session.AllowedCommands)
	if err != nil {
		return Session{}, err
	}
	patch, files, err := validateProposalPatch(input.Patch, session.AllowedPaths)
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	expectedRevision := session.Revision
	session.Status = StatusAwaitingApproval
	session.Revision++
	session.UpdatedAt = now
	session.Proposal = &Proposal{
		Summary: input.Summary, PatchHash: patchHash(session.BaselineCommit, patch, commandIDs), PatchBytes: len(patch),
		Files: files, Commands: commands, SubmittedBy: actor, SubmittedAt: now, Patch: patch,
	}
	event := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: EventProposalSubmitted, FromStatus: StatusDraft, ToStatus: session.Status, Actor: actor, Reason: input.Summary, CreatedAt: now}
	if err := s.store.Update(ctx, session, expectedRevision, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Decide(ctx context.Context, id string, input DecisionInput, actor string) (Session, error) {
	session, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Session{}, err
	}
	actor = strings.TrimSpace(actor)
	input.Decision = strings.TrimSpace(input.Decision)
	input.Comment = strings.TrimSpace(input.Comment)
	if actor == "" {
		return Session{}, ErrForbiddenActor
	}
	if actor == session.CreatedBy {
		return Session{}, ErrSelfApproval
	}
	if input.Revision != session.Revision {
		return Session{}, ErrRevisionConflict
	}
	if session.Status != StatusAwaitingApproval || session.Proposal == nil || (input.Decision != "approve" && input.Decision != "reject") || input.Comment == "" || len(input.Comment) > 4000 || !utf8.ValidString(input.Comment) {
		return Session{}, ErrInvalidTransition
	}
	if err := s.validateBaseline(ctx, session); err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	expectedRevision := session.Revision
	from := session.Status
	eventType := EventRejected
	session.Status = StatusRejected
	if input.Decision == "approve" {
		eventType = EventApproved
		session.Status = StatusApproved
	}
	session.Revision++
	session.UpdatedAt = now
	session.ReviewedBy = actor
	session.ReviewedAt = &now
	session.ReviewComment = input.Comment
	event := Event{ID: newID(), SessionID: session.ID, Sequence: session.Revision, Type: eventType, FromStatus: from, ToStatus: session.Status, Actor: actor, Reason: input.Comment, CreatedAt: now}
	if err := s.store.Update(ctx, session, expectedRevision, event); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) Get(ctx context.Context, id string) (Session, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, project string, limit int) ([]Session, error) {
	project = strings.ToLower(strings.TrimSpace(project))
	if project == "" {
		return nil, ErrInvalidInput
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.List(ctx, project, limit)
}

func (s *Service) Events(ctx context.Context, id string) ([]Event, error) {
	return s.store.ListEvents(ctx, strings.TrimSpace(id))
}

func (s *Service) Preview(ctx context.Context, id string) (Preview, error) {
	session, err := s.Get(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	if session.Proposal == nil {
		return Preview{}, ErrInvalidTransition
	}
	return Preview{SessionID: session.ID, Revision: session.Revision, BaselineCommit: session.BaselineCommit, PatchHash: session.Proposal.PatchHash, Patch: session.Proposal.Patch, Files: append([]FileChange(nil), session.Proposal.Files...), Commands: cloneCommands(session.Proposal.Commands)}, nil
}

func (s *Service) validateBaseline(ctx context.Context, session Session) error {
	state, err := s.workspace.Inspect(ctx, session.Repository)
	if err != nil {
		return err
	}
	if state.Dirty {
		return ErrDirtyWorkspace
	}
	if state.HeadCommit != session.BaselineCommit {
		return ErrBaselineChanged
	}
	return nil
}

func (s *Service) requireApprovedPackage(ctx context.Context, projectPackage initiative.Package) error {
	reviews, err := s.initiative.Reviews(ctx, projectPackage.ID, "", 200)
	if err != nil {
		return err
	}
	latest := make(map[string]initiative.ArtifactReview)
	for _, review := range reviews {
		if _, found := latest[review.ArtifactType]; !found {
			latest[review.ArtifactType] = review
		}
	}
	for _, artifactType := range initiative.RequiredArtifactTypes() {
		review, found := latest[artifactType]
		if !found || review.Decision != "approve" || review.PackageHash != projectPackage.DefinitionHash {
			return ErrPackageNotApproved
		}
	}
	for _, record := range projectPackage.Traceability {
		if record.CoverageStatus != "covered" {
			return ErrPackageNotApproved
		}
	}
	return nil
}

func resolveCommands(requested, allowed []string) ([]Command, []string, error) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	commands := make([]Command, 0, len(requested))
	ids := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if _, found := allowedSet[id]; !found {
			return nil, nil, ErrCommandNotAllowed
		}
		if _, found := seen[id]; found {
			return nil, nil, ErrInvalidInput
		}
		seen[id] = struct{}{}
		command := commandCatalog[id]
		command.Arguments = append([]string(nil), command.Arguments...)
		commands = append(commands, command)
		ids = append(ids, id)
	}
	return commands, ids, nil
}

func cloneCommands(values []Command) []Command {
	result := append([]Command(nil), values...)
	for index := range result {
		result[index].Arguments = append([]string(nil), result[index].Arguments...)
	}
	return result
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))[:32]
	}
	return hex.EncodeToString(value[:])
}
