package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ekbda/internal/agenttask"
	"ekbda/internal/planning"
)

func (s *Server) createRoleReviewTask(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var request struct {
		SessionID string `json:"session_id"`
		Revision  int    `json:"revision"`
	}
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.SessionID) == "" || request.Revision < 1 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	session, err := s.planning.Get(r.Context(), request.SessionID)
	if err != nil {
		s.writePlanningError(w, err, "failed to load planning session")
		return
	}
	if !s.authorizeProject(w, r, session.Project, session.Repository) {
		return
	}
	governanceOverride := requestHasRole(r, "project_approver") || requestHasRole(r, "knowledge_admin")
	if userID(r) != session.CreatedBy && !governanceOverride {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: planning.ErrForbiddenParticipant.Error()})
		return
	}
	if session.Status != planning.StatusAwaitingRoleReview || request.Revision != session.Revision {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "planning session is not ready for this role-review task"})
		return
	}
	task, err := s.agentTasks.Create(r.Context(), agenttask.KindRoleReview, session.Project, session.Repository, agenttask.RoleReviewInput{
		SessionID: session.ID, Revision: request.Revision, Roles: roles(r), GovernanceOverride: governanceOverride,
	}, userID(r))
	if err != nil {
		s.writeAgentTaskError(w, err, "failed to create role-review task")
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) createProjectPackageTask(w http.ResponseWriter, r *http.Request) {
	if !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input agenttask.ProjectPackageInput
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.ChangeSummary) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	session, err := s.planning.Get(r.Context(), input.SessionID)
	if err != nil {
		s.writePlanningError(w, err, "failed to load planning session")
		return
	}
	if !s.authorizeProject(w, r, session.Project, session.Repository) {
		return
	}
	if session.Status != planning.StatusApproved {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "planning session must be approved before creating a project-package task"})
		return
	}
	task, err := s.agentTasks.Create(r.Context(), agenttask.KindProjectPackage, session.Project, session.Repository, input, userID(r))
	if err != nil {
		s.writeAgentTaskError(w, err, "failed to create project-package task")
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) getAgentTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.loadAuthorizedAgentTask(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) listAgentTasks(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	if project == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "project is required"})
		return
	}
	if !s.authorizeProject(w, r, project, "") {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tasks, err := s.agentTasks.List(r.Context(), project, r.URL.Query().Get("kind"), r.URL.Query().Get("status"), limit)
	if err != nil {
		s.writeAgentTaskError(w, err, "failed to list agent tasks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) cancelAgentTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.loadAuthorizedAgentTask(w, r)
	if !ok {
		return
	}
	if task.TriggeredBy != userID(r) && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "only the task creator or knowledge_admin may cancel this task"})
		return
	}
	updated, err := s.agentTasks.Cancel(r.Context(), task.ID)
	if err != nil {
		s.writeAgentTaskError(w, err, "failed to cancel agent task")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) retryAgentTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.loadAuthorizedAgentTask(w, r)
	if !ok {
		return
	}
	if task.TriggeredBy != userID(r) && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "only the task creator or knowledge_admin may retry this task"})
		return
	}
	if task.Kind == agenttask.KindProjectPackage && !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	retry, err := s.agentTasks.Retry(r.Context(), task.ID, userID(r))
	if err != nil {
		s.writeAgentTaskError(w, err, "failed to retry agent task")
		return
	}
	writeJSON(w, http.StatusAccepted, retry)
}

func (s *Server) loadAuthorizedAgentTask(w http.ResponseWriter, r *http.Request) (agenttask.Task, bool) {
	task, err := s.agentTasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeAgentTaskError(w, err, "failed to load agent task")
		return agenttask.Task{}, false
	}
	if !s.authorizeProject(w, r, task.Project, task.Repository) {
		return agenttask.Task{}, false
	}
	return task, true
}

func (s *Server) writeAgentTaskError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, agenttask.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, agenttask.ErrTaskNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, agenttask.ErrTaskNotRetryable), errors.Is(err, agenttask.ErrTaskAlreadyRetried):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, agenttask.ErrExecutorUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fallback})
	}
}
