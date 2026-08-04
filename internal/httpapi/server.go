package httpapi

import (
	"net/http"

	"ekbda/internal/access"
	"ekbda/internal/agenttask"
	"ekbda/internal/answer"
	"ekbda/internal/auth"
	"ekbda/internal/development"
	"ekbda/internal/evaluation"
	"ekbda/internal/ingestion"
	"ekbda/internal/initiative"
	"ekbda/internal/knowledge"
	"ekbda/internal/planning"
	"ekbda/internal/release"
	"ekbda/internal/repositorysync"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

type Server struct {
	knowledge      *knowledge.Service
	ingestion      *ingestion.Service
	answer         *answer.Service
	evaluation     *evaluation.Service
	standards      *standards.Service
	workspace      *workspace.Service
	access         *access.Service
	repositorySync *repositorysync.Service
	planning       *planning.Service
	initiative     *initiative.Service
	agentTasks     *agenttask.Service
	development    *development.Service
	releases       *release.Service
	codeWebhook    *release.WebhookVerifier
	releaseWebhook *release.WebhookVerifier
	auth           auth.Authenticator
	mux            *http.ServeMux
}

func New(knowledgeService *knowledge.Service, ingestionService *ingestion.Service, answerService *answer.Service, evaluationService *evaluation.Service, standardsService *standards.Service, workspaceService *workspace.Service, accessService *access.Service, repositorySyncService *repositorysync.Service, planningService *planning.Service, initiativeService *initiative.Service, authenticators ...auth.Authenticator) http.Handler {
	return NewWithAgentTasks(knowledgeService, ingestionService, answerService, evaluationService, standardsService, workspaceService, accessService, repositorySyncService, planningService, initiativeService, nil, authenticators...)
}

func NewWithAgentTasks(knowledgeService *knowledge.Service, ingestionService *ingestion.Service, answerService *answer.Service, evaluationService *evaluation.Service, standardsService *standards.Service, workspaceService *workspace.Service, accessService *access.Service, repositorySyncService *repositorysync.Service, planningService *planning.Service, initiativeService *initiative.Service, agentTaskService *agenttask.Service, authenticators ...auth.Authenticator) http.Handler {
	return NewWithDevelopment(knowledgeService, ingestionService, answerService, evaluationService, standardsService, workspaceService, accessService, repositorySyncService, planningService, initiativeService, agentTaskService, nil, authenticators...)
}

func NewWithDevelopment(knowledgeService *knowledge.Service, ingestionService *ingestion.Service, answerService *answer.Service, evaluationService *evaluation.Service, standardsService *standards.Service, workspaceService *workspace.Service, accessService *access.Service, repositorySyncService *repositorysync.Service, planningService *planning.Service, initiativeService *initiative.Service, agentTaskService *agenttask.Service, developmentService *development.Service, authenticators ...auth.Authenticator) http.Handler {
	return NewWithRelease(knowledgeService, ingestionService, answerService, evaluationService, standardsService, workspaceService, accessService, repositorySyncService, planningService, initiativeService, agentTaskService, developmentService, nil, nil, authenticators...)
}

func NewWithRelease(knowledgeService *knowledge.Service, ingestionService *ingestion.Service, answerService *answer.Service, evaluationService *evaluation.Service, standardsService *standards.Service, workspaceService *workspace.Service, accessService *access.Service, repositorySyncService *repositorysync.Service, planningService *planning.Service, initiativeService *initiative.Service, agentTaskService *agenttask.Service, developmentService *development.Service, releaseService *release.Service, releaseWebhook *release.WebhookVerifier, authenticators ...auth.Authenticator) http.Handler {
	return NewWithReleaseWebhooks(knowledgeService, ingestionService, answerService, evaluationService, standardsService, workspaceService, accessService, repositorySyncService, planningService, initiativeService, agentTaskService, developmentService, releaseService, nil, releaseWebhook, authenticators...)
}

func NewWithReleaseWebhooks(knowledgeService *knowledge.Service, ingestionService *ingestion.Service, answerService *answer.Service, evaluationService *evaluation.Service, standardsService *standards.Service, workspaceService *workspace.Service, accessService *access.Service, repositorySyncService *repositorysync.Service, planningService *planning.Service, initiativeService *initiative.Service, agentTaskService *agenttask.Service, developmentService *development.Service, releaseService *release.Service, codeWebhook *release.WebhookVerifier, releaseWebhook *release.WebhookVerifier, authenticators ...auth.Authenticator) http.Handler {
	authenticator := auth.Authenticator(auth.NewDevHeaders())
	if len(authenticators) > 0 && authenticators[0] != nil {
		authenticator = authenticators[0]
	}
	server := &Server{
		knowledge:      knowledgeService,
		ingestion:      ingestionService,
		answer:         answerService,
		evaluation:     evaluationService,
		standards:      standardsService,
		workspace:      workspaceService,
		access:         accessService,
		repositorySync: repositorySyncService,
		planning:       planningService,
		initiative:     initiativeService,
		agentTasks:     agentTaskService,
		development:    developmentService,
		releases:       releaseService,
		codeWebhook:    codeWebhook,
		releaseWebhook: releaseWebhook,
		auth:           authenticator,
		mux:            http.NewServeMux(),
	}
	server.routes()
	return server.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("POST /api/v1/access/projects", s.requireIdentity(s.requireRole("knowledge_admin", s.createAccessPolicy)))
	s.mux.HandleFunc("GET /api/v1/access/projects/{project}", s.requireIdentity(s.requireRole("knowledge_admin", s.getAccessPolicy)))
	s.mux.HandleFunc("GET /api/v1/access/projects/{project}/versions", s.requireIdentity(s.requireRole("knowledge_admin", s.listAccessPolicies)))
	s.mux.HandleFunc("POST /api/v1/repositories/syncs", s.requireIdentity(s.createRepositorySync))
	s.mux.HandleFunc("GET /api/v1/repositories/syncs", s.requireIdentity(s.requireRole("knowledge_admin", s.listRepositorySyncs)))
	s.mux.HandleFunc("GET /api/v1/repositories/syncs/{id}", s.requireIdentity(s.requireRole("knowledge_admin", s.getRepositorySync)))
	s.mux.HandleFunc("POST /api/v1/planning/sessions", s.requireIdentity(s.createPlanningSession))
	s.mux.HandleFunc("GET /api/v1/planning/sessions", s.requireIdentity(s.listPlanningSessions))
	s.mux.HandleFunc("GET /api/v1/planning/sessions/{id}", s.requireIdentity(s.getPlanningSession))
	s.mux.HandleFunc("GET /api/v1/planning/sessions/{id}/events", s.requireIdentity(s.listPlanningEvents))
	s.mux.HandleFunc("POST /api/v1/planning/sessions/{id}/clarifications", s.requireIdentity(s.submitPlanningClarifications))
	s.mux.HandleFunc("POST /api/v1/planning/sessions/{id}/role-reviews", s.requireIdentity(s.submitPlanningRoleReviews))
	s.mux.HandleFunc("POST /api/v1/planning/sessions/{id}/resolutions", s.requireIdentity(s.resolvePlanningReviewDecisions))
	s.mux.HandleFunc("POST /api/v1/planning/sessions/{id}/decision", s.requireIdentity(s.decidePlanningSession))
	s.mux.HandleFunc("POST /api/v1/project-packages", s.requireIdentity(s.createProjectPackage))
	s.mux.HandleFunc("GET /api/v1/project-packages", s.requireIdentity(s.listProjectPackages))
	s.mux.HandleFunc("GET /api/v1/project-packages/{id}", s.requireIdentity(s.getProjectPackage))
	s.mux.HandleFunc("POST /api/v1/project-packages/{id}/reviews", s.requireIdentity(s.createProjectPackageReview))
	s.mux.HandleFunc("GET /api/v1/project-packages/{id}/reviews", s.requireIdentity(s.listProjectPackageReviews))
	s.mux.HandleFunc("GET /api/v1/project-packages/{id}/export", s.requireIdentity(s.exportProjectPackage))
	if s.agentTasks != nil {
		s.mux.HandleFunc("POST /api/v1/agent-tasks/role-reviews", s.requireIdentity(s.createRoleReviewTask))
		s.mux.HandleFunc("POST /api/v1/agent-tasks/project-packages", s.requireIdentity(s.createProjectPackageTask))
		s.mux.HandleFunc("GET /api/v1/agent-tasks", s.requireIdentity(s.listAgentTasks))
		s.mux.HandleFunc("GET /api/v1/agent-tasks/{id}", s.requireIdentity(s.getAgentTask))
		s.mux.HandleFunc("POST /api/v1/agent-tasks/{id}/cancel", s.requireIdentity(s.cancelAgentTask))
		s.mux.HandleFunc("POST /api/v1/agent-tasks/{id}/retry", s.requireIdentity(s.retryAgentTask))
	}
	if s.development != nil {
		s.mux.HandleFunc("GET /api/v1/development/commands", s.requireIdentity(s.listDevelopmentCommands))
		s.mux.HandleFunc("POST /api/v1/development/sessions", s.requireIdentity(s.createDevelopmentSession))
		s.mux.HandleFunc("GET /api/v1/development/sessions", s.requireIdentity(s.listDevelopmentSessions))
		s.mux.HandleFunc("GET /api/v1/development/sessions/{id}", s.requireIdentity(s.getDevelopmentSession))
		s.mux.HandleFunc("GET /api/v1/development/sessions/{id}/events", s.requireIdentity(s.listDevelopmentEvents))
		s.mux.HandleFunc("GET /api/v1/development/sessions/{id}/preview", s.requireIdentity(s.getDevelopmentPreview))
		s.mux.HandleFunc("POST /api/v1/development/sessions/{id}/proposals", s.requireIdentity(s.submitDevelopmentProposal))
		s.mux.HandleFunc("POST /api/v1/development/sessions/{id}/decision", s.requireIdentity(s.decideDevelopmentSession))
		s.mux.HandleFunc("POST /api/v1/development/sessions/{id}/execute", s.requireIdentity(s.executeDevelopmentSession))
		s.mux.HandleFunc("POST /api/v1/development/sessions/{id}/deliver", s.requireIdentity(s.deliverDevelopmentSession))
	}
	if s.releases != nil {
		s.mux.HandleFunc("GET /api/v1/releases/catalog", s.requireIdentity(s.getReleaseCatalog))
		s.mux.HandleFunc("POST /api/v1/releases", s.requireIdentity(s.createRelease))
		s.mux.HandleFunc("GET /api/v1/releases", s.requireIdentity(s.listReleases))
		s.mux.HandleFunc("GET /api/v1/releases/{id}", s.requireIdentity(s.getRelease))
		s.mux.HandleFunc("GET /api/v1/releases/{id}/events", s.requireIdentity(s.listReleaseEvents))
		s.mux.HandleFunc("POST /api/v1/releases/{id}/decision", s.requireIdentity(s.decideRelease))
		s.mux.HandleFunc("POST /api/v1/releases/{id}/trigger", s.requireIdentity(s.triggerRelease))
		s.mux.HandleFunc("POST /api/v1/releases/{id}/rollback", s.requireIdentity(s.requestReleaseRollback))
		s.mux.HandleFunc("POST /api/v1/releases/{id}/rollback-decision", s.requireIdentity(s.decideReleaseRollback))
		s.mux.HandleFunc("POST /api/v1/releases/{id}/rollback-trigger", s.requireIdentity(s.triggerReleaseRollback))
		s.mux.HandleFunc("POST /api/v1/releases/webhooks/provider", s.releaseProviderWebhook)
		s.mux.HandleFunc("POST /api/v1/releases/webhooks/code-platform", s.releaseCodePlatformWebhook)
	}
	s.mux.HandleFunc("POST /api/v1/knowledge/documents", s.requireIdentity(s.requireRole("knowledge_admin", s.createDocument)))
	s.mux.HandleFunc("GET /api/v1/knowledge/documents/{id}/versions", s.requireIdentity(s.requireRole("knowledge_admin", s.listDocumentVersions)))
	s.mux.HandleFunc("POST /api/v1/knowledge/imports", s.requireIdentity(s.requireRole("knowledge_admin", s.createImport)))
	s.mux.HandleFunc("GET /api/v1/knowledge/imports/{id}", s.requireIdentity(s.requireRole("knowledge_admin", s.getImport)))
	s.mux.HandleFunc("GET /api/v1/knowledge/search", s.requireIdentity(s.search))
	s.mux.HandleFunc("POST /api/v1/knowledge/answers", s.requireIdentity(s.createAnswer))
	s.mux.HandleFunc("GET /api/v1/observability/answer-traces/{id}", s.requireIdentity(s.requireRole("knowledge_admin", s.getAnswerTrace)))
	s.mux.HandleFunc("POST /api/v1/observability/answer-traces/prune", s.requireIdentity(s.requireRole("knowledge_admin", s.pruneAnswerTraces)))
	s.mux.HandleFunc("GET /api/v1/observability/answer-metrics", s.requireIdentity(s.requireRole("knowledge_admin", s.getAnswerMetrics)))
	s.mux.HandleFunc("POST /api/v1/evaluations/answers", s.requireIdentity(s.requireRole("knowledge_admin", s.runAnswerEvaluation)))
	s.mux.HandleFunc("POST /api/v1/evaluations/suites", s.requireIdentity(s.requireRole("knowledge_admin", s.createEvaluationSuite)))
	s.mux.HandleFunc("GET /api/v1/evaluations/suites", s.requireIdentity(s.requireRole("knowledge_admin", s.listEvaluationSuites)))
	s.mux.HandleFunc("GET /api/v1/evaluations/suites/{id}", s.requireIdentity(s.requireRole("knowledge_admin", s.getEvaluationSuite)))
	s.mux.HandleFunc("POST /api/v1/evaluations/runs", s.requireIdentity(s.requireRole("knowledge_admin", s.createEvaluationRun)))
	s.mux.HandleFunc("GET /api/v1/evaluations/runs", s.requireIdentity(s.requireRole("knowledge_admin", s.listEvaluationRuns)))
	s.mux.HandleFunc("GET /api/v1/evaluations/runs/{id}", s.requireIdentity(s.requireRole("knowledge_admin", s.getEvaluationRun)))
	s.mux.HandleFunc("POST /api/v1/evaluations/runs/{id}/cancel", s.requireIdentity(s.requireRole("knowledge_admin", s.cancelEvaluationRun)))
	s.mux.HandleFunc("POST /api/v1/evaluations/runs/{id}/retry", s.requireIdentity(s.requireRole("knowledge_admin", s.retryEvaluationRun)))
	s.mux.HandleFunc("POST /api/v1/standards/packages", s.requireIdentity(s.requireRole("knowledge_admin", s.createStandardPackage)))
	s.mux.HandleFunc("GET /api/v1/standards/packages", s.requireIdentity(s.listStandardPackages))
	s.mux.HandleFunc("GET /api/v1/standards/packages/{id}", s.requireIdentity(s.getStandardPackage))
	s.mux.HandleFunc("POST /api/v1/standards/validations", s.requireIdentity(s.validateStandards))
	s.mux.HandleFunc("GET /api/v1/standards/validations", s.requireIdentity(s.requireRole("knowledge_admin", s.listStandardReports)))
	s.mux.HandleFunc("GET /api/v1/standards/validations/{id}", s.requireIdentity(s.requireRole("knowledge_admin", s.getStandardReport)))
	s.mux.HandleFunc("POST /api/v1/workspaces/validations", s.requireIdentity(s.validateWorkspace))
	s.mux.HandleFunc("GET /api/v1/workspaces/validations", s.requireIdentity(s.requireRole("knowledge_admin", s.listWorkspaceValidations)))
	s.mux.HandleFunc("GET /api/v1/workspaces/validations/{id}", s.requireIdentity(s.requireRole("knowledge_admin", s.getWorkspaceValidation)))
	s.mux.Handle("GET /", s.frontend())
}
