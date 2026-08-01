package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

func (s *Server) validateWorkspace(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input workspace.ValidateInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	if !s.authorizeProject(w, r, input.Project, input.Repository) {
		return
	}
	result, err := s.workspace.Validate(r.Context(), input, userID(r))
	if err != nil {
		switch {
		case errors.Is(err, workspace.ErrDisabled):
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
		case errors.Is(err, workspace.ErrInvalidRepository), errors.Is(err, standards.ErrInvalidValidation):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		case errors.Is(err, workspace.ErrRepositoryChanged):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case errors.Is(err, workspace.ErrRepositoryTooLarge):
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: err.Error()})
		case errors.Is(err, standards.ErrNoApplicablePackages), errors.Is(err, standards.ErrApplicableRuleConflict):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate Git workspace"})
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getWorkspaceValidation(w http.ResponseWriter, r *http.Request) {
	result, err := s.workspace.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, workspace.ErrSnapshotNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load workspace validation"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listWorkspaceValidations(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	snapshots, err := s.workspace.List(r.Context(), r.URL.Query().Get("project"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list workspace validations"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"validations": snapshots})
}
