package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ekbda/internal/planning"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

func (s *Server) createPlanningSession(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input planning.CreateInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	if !s.authorizeProject(w, r, input.Project, input.Repository) {
		return
	}
	session, err := s.planning.Create(r.Context(), input, userID(r), roles(r))
	if err != nil {
		s.writePlanningError(w, err, "failed to create planning session")
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) getPlanningSession(w http.ResponseWriter, r *http.Request) {
	session, ok := s.loadAuthorizedPlanningSession(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) listPlanningSessions(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	if project == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "project is required"})
		return
	}
	if !s.authorizeProject(w, r, project, "") {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sessions, err := s.planning.List(r.Context(), project, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list planning sessions"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) listPlanningEvents(w http.ResponseWriter, r *http.Request) {
	session, ok := s.loadAuthorizedPlanningSession(w, r)
	if !ok {
		return
	}
	events, err := s.planning.Events(r.Context(), session.ID)
	if err != nil {
		s.writePlanningError(w, err, "failed to list planning session events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) submitPlanningClarifications(w http.ResponseWriter, r *http.Request) {
	session, ok := s.loadAuthorizedPlanningSession(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input planning.ClarificationInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	updated, err := s.planning.SubmitClarifications(r.Context(), session.ID, input, userID(r), roles(r), requestHasRole(r, "knowledge_admin"))
	if err != nil {
		s.writePlanningError(w, err, "failed to submit planning clarifications")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) submitPlanningRoleReviews(w http.ResponseWriter, r *http.Request) {
	session, ok := s.loadAuthorizedPlanningSession(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input struct {
		Revision int `json:"revision"`
	}
	if err := decoder.Decode(&input); err != nil || input.Revision < 1 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	governanceOverride := requestHasRole(r, "project_approver") || requestHasRole(r, "knowledge_admin")
	updated, err := s.planning.SubmitRoleReviews(r.Context(), session.ID, input.Revision, userID(r), roles(r), governanceOverride)
	if err != nil {
		s.writePlanningError(w, err, "failed to complete planning role reviews")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) resolvePlanningReviewDecisions(w http.ResponseWriter, r *http.Request) {
	session, ok := s.loadAuthorizedPlanningSession(w, r)
	if !ok {
		return
	}
	if !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input planning.ResolutionInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	updated, err := s.planning.ResolveReviewDecisions(r.Context(), session.ID, input, userID(r))
	if err != nil {
		s.writePlanningError(w, err, "failed to resolve planning review decisions")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) decidePlanningSession(w http.ResponseWriter, r *http.Request) {
	session, ok := s.loadAuthorizedPlanningSession(w, r)
	if !ok {
		return
	}
	if !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input planning.DecisionInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	updated, err := s.planning.Decide(r.Context(), session.ID, input, userID(r))
	if err != nil {
		s.writePlanningError(w, err, "failed to decide planning session")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) loadAuthorizedPlanningSession(w http.ResponseWriter, r *http.Request) (planning.Session, bool) {
	session, err := s.planning.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writePlanningError(w, err, "failed to load planning session")
		return planning.Session{}, false
	}
	if !s.authorizeProject(w, r, session.Project, session.Repository) {
		return planning.Session{}, false
	}
	return session, true
}

func (s *Server) writePlanningError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, planning.ErrInvalidInput), errors.Is(err, planning.ErrIncompleteAnswers), errors.Is(err, planning.ErrIncompleteResolutions), errors.Is(err, standards.ErrInvalidValidation), errors.Is(err, workspace.ErrInvalidRepository):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, planning.ErrSessionNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, planning.ErrRevisionConflict), errors.Is(err, planning.ErrInvalidTransition), errors.Is(err, workspace.ErrRepositoryChanged):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, planning.ErrForbiddenParticipant), errors.Is(err, planning.ErrSelfApproval), errors.Is(err, planning.ErrSelfResolution):
		writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error()})
	case errors.Is(err, workspace.ErrDisabled):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	case errors.Is(err, workspace.ErrRepositoryTooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: err.Error()})
	case errors.Is(err, planning.ErrProviderFailed), errors.Is(err, planning.ErrInvalidProviderOutput):
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fallback})
	}
}

func requestHasRole(r *http.Request, expected string) bool {
	for _, role := range roles(r) {
		if role == expected {
			return true
		}
	}
	return false
}
