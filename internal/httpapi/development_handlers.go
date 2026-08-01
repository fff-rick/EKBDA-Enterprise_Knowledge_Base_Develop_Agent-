package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ekbda/internal/development"
	"ekbda/internal/initiative"
	"ekbda/internal/workspace"
)

func (s *Server) listDevelopmentCommands(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"commands": development.CommandCatalog(), "execution_enabled": s.development.ExecutionEnabled(),
		"delivery_enabled": s.development.DeliveryEnabled(),
	})
}

func (s *Server) deliverDevelopmentSession(w http.ResponseWriter, r *http.Request) {
	if !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	session, ok := s.authorizedDevelopmentSession(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input development.DeliverInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	updated, err := s.development.Deliver(r.Context(), session.ID, input, userID(r))
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to deliver verified development proposal")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) executeDevelopmentSession(w http.ResponseWriter, r *http.Request) {
	if !requestHasRole(r, "developer") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: developer"})
		return
	}
	session, ok := s.authorizedDevelopmentSession(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input development.ExecuteInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	updated, err := s.development.Execute(r.Context(), session.ID, input, userID(r))
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to execute approved development proposal")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) createDevelopmentSession(w http.ResponseWriter, r *http.Request) {
	if !requestHasRole(r, "developer") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: developer"})
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input development.CreateInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	projectPackage, err := s.initiative.Get(r.Context(), input.ProjectPackageID)
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to load project package")
		return
	}
	if !s.authorizeProject(w, r, projectPackage.Project, projectPackage.Repository) {
		return
	}
	session, err := s.development.Create(r.Context(), input, userID(r))
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to create development session")
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) listDevelopmentSessions(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	if project == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "project is required"})
		return
	}
	if !s.authorizeProject(w, r, project, "") {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sessions, err := s.development.List(r.Context(), project, limit)
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to list development sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) getDevelopmentSession(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authorizedDevelopmentSession(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) listDevelopmentEvents(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authorizedDevelopmentSession(w, r)
	if !ok {
		return
	}
	events, err := s.development.Events(r.Context(), session.ID)
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to list development events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) getDevelopmentPreview(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authorizedDevelopmentSession(w, r)
	if !ok {
		return
	}
	preview, err := s.development.Preview(r.Context(), session.ID)
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to load development preview")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) submitDevelopmentProposal(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authorizedDevelopmentSession(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, (512<<10)+(64<<10)))
	decoder.DisallowUnknownFields()
	var input development.SubmitInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	updated, err := s.development.Submit(r.Context(), session.ID, input, userID(r))
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to submit development proposal")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) decideDevelopmentSession(w http.ResponseWriter, r *http.Request) {
	if !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	session, ok := s.authorizedDevelopmentSession(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input development.DecisionInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	updated, err := s.development.Decide(r.Context(), session.ID, input, userID(r))
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to decide development session")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) authorizedDevelopmentSession(w http.ResponseWriter, r *http.Request) (development.Session, bool) {
	session, err := s.development.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeDevelopmentError(w, err, "failed to load development session")
		return development.Session{}, false
	}
	if !s.authorizeProject(w, r, session.Project, session.Repository) {
		return development.Session{}, false
	}
	return session, true
}

func (s *Server) writeDevelopmentError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, development.ErrInvalidInput), errors.Is(err, development.ErrInvalidPatch):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, development.ErrSessionNotFound), errors.Is(err, initiative.ErrPackageNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, development.ErrForbiddenActor), errors.Is(err, development.ErrSelfApproval), errors.Is(err, development.ErrSelfDelivery):
		writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error()})
	case errors.Is(err, development.ErrRevisionConflict), errors.Is(err, development.ErrInvalidTransition), errors.Is(err, development.ErrDirtyWorkspace), errors.Is(err, development.ErrMissingBaseline), errors.Is(err, development.ErrBaselineChanged), errors.Is(err, development.ErrPackageNotApproved), errors.Is(err, development.ErrPathNotAllowed), errors.Is(err, development.ErrSensitiveContent), errors.Is(err, development.ErrCommandNotAllowed), errors.Is(err, development.ErrExecutionConflict), errors.Is(err, development.ErrDeliveryConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, development.ErrExecutionDisabled), errors.Is(err, development.ErrDeliveryDisabled):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	case errors.Is(err, workspace.ErrInvalidRepository), errors.Is(err, workspace.ErrDisabled):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fallback})
	}
}
