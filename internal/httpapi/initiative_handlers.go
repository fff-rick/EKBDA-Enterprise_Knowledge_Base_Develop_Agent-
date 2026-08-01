package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ekbda/internal/initiative"
	"ekbda/internal/planning"
)

func (s *Server) createProjectPackage(w http.ResponseWriter, r *http.Request) {
	if !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input initiative.CreateInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	session, err := s.planning.Get(r.Context(), input.SessionID)
	if err != nil {
		s.writeInitiativeError(w, err, "failed to load planning session")
		return
	}
	if !s.authorizeProject(w, r, session.Project, session.Repository) {
		return
	}
	projectPackage, err := s.initiative.Create(r.Context(), input, userID(r))
	if err != nil {
		s.writeInitiativeError(w, err, "failed to create project package")
		return
	}
	writeJSON(w, http.StatusCreated, projectPackage)
}

func (s *Server) getProjectPackage(w http.ResponseWriter, r *http.Request) {
	projectPackage, err := s.initiative.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeInitiativeError(w, err, "failed to load project package")
		return
	}
	if !s.authorizeProject(w, r, projectPackage.Project, projectPackage.Repository) {
		return
	}
	writeJSON(w, http.StatusOK, projectPackage)
}

func (s *Server) listProjectPackages(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	if project == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "project is required"})
		return
	}
	if !s.authorizeProject(w, r, project, "") {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	packages, err := s.initiative.List(r.Context(), project, r.URL.Query().Get("name"), limit)
	if err != nil {
		s.writeInitiativeError(w, err, "failed to list project packages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": packages})
}

func (s *Server) createProjectPackageReview(w http.ResponseWriter, r *http.Request) {
	if !requestHasRole(r, "project_approver") && !requestHasRole(r, "knowledge_admin") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: project_approver"})
		return
	}
	projectPackage, err := s.initiative.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeInitiativeError(w, err, "failed to load project package")
		return
	}
	if !s.authorizeProject(w, r, projectPackage.Project, projectPackage.Repository) {
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input initiative.ReviewInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	review, err := s.initiative.Review(r.Context(), projectPackage.ID, input, userID(r))
	if err != nil {
		s.writeInitiativeError(w, err, "failed to review project package artifact")
		return
	}
	writeJSON(w, http.StatusCreated, review)
}

func (s *Server) listProjectPackageReviews(w http.ResponseWriter, r *http.Request) {
	projectPackage, err := s.initiative.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeInitiativeError(w, err, "failed to load project package")
		return
	}
	if !s.authorizeProject(w, r, projectPackage.Project, projectPackage.Repository) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reviews, err := s.initiative.Reviews(r.Context(), projectPackage.ID, r.URL.Query().Get("artifact_type"), limit)
	if err != nil {
		s.writeInitiativeError(w, err, "failed to list project package reviews")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews})
}

func (s *Server) exportProjectPackage(w http.ResponseWriter, r *http.Request) {
	projectPackage, err := s.initiative.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeInitiativeError(w, err, "failed to load project package")
		return
	}
	if !s.authorizeProject(w, r, projectPackage.Project, projectPackage.Repository) {
		return
	}
	document, err := s.initiative.Export(r.Context(), projectPackage.ID, r.URL.Query().Get("format"))
	if err != nil {
		s.writeInitiativeError(w, err, "failed to export project package")
		return
	}
	w.Header().Set("Content-Type", document.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+document.Filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document.Data)
}

func (s *Server) writeInitiativeError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, initiative.ErrInvalidInput), errors.Is(err, initiative.ErrInvalidReview):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, initiative.ErrPackageNotFound), errors.Is(err, planning.ErrSessionNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, initiative.ErrPlanningNotApproved):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, initiative.ErrPackageHashConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, initiative.ErrProviderFailed), errors.Is(err, initiative.ErrInvalidProviderOutput):
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fallback})
	}
}
