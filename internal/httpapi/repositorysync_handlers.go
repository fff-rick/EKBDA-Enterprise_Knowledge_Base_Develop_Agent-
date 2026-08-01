package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ekbda/internal/repositorysync"
	"ekbda/internal/workspace"
)

func (s *Server) createRepositorySync(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input repositorysync.Input
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	if !s.authorizeProject(w, r, input.Project, input.Repository) {
		return
	}
	report, err := s.repositorySync.Sync(r.Context(), input, userID(r))
	if err != nil {
		switch {
		case errors.Is(err, repositorysync.ErrInvalidInput), errors.Is(err, workspace.ErrInvalidRepository):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		case errors.Is(err, repositorysync.ErrDirtyRepository), errors.Is(err, repositorysync.ErrSyncInProgress), errors.Is(err, workspace.ErrRepositoryChanged):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case errors.Is(err, workspace.ErrDisabled):
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
		case errors.Is(err, workspace.ErrRepositoryTooLarge):
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to synchronize repository knowledge"})
		}
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) getRepositorySync(w http.ResponseWriter, r *http.Request) {
	report, err := s.repositorySync.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, repositorysync.ErrReportNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load repository knowledge sync report"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) listRepositorySyncs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reports, err := s.repositorySync.List(r.Context(), r.URL.Query().Get("project"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list repository knowledge sync reports"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"syncs": reports})
}
