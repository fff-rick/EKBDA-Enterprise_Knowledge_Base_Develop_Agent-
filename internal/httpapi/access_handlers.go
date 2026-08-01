package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ekbda/internal/access"
	"ekbda/internal/auth"
)

func (s *Server) createAccessPolicy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input access.CreatePolicyInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	policy, err := s.access.CreatePolicy(r.Context(), input, userID(r))
	if err != nil {
		if errors.Is(err, access.ErrInvalidPolicy) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to publish project access policy"})
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

func (s *Server) getAccessPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := s.access.GetLatest(r.Context(), r.PathValue("project"))
	if err != nil {
		switch {
		case errors.Is(err, access.ErrInvalidPolicy):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		case errors.Is(err, access.ErrPolicyNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load project access policy"})
		}
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) listAccessPolicies(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	policies, err := s.access.ListPolicies(r.Context(), r.PathValue("project"), limit)
	if err != nil {
		if errors.Is(err, access.ErrInvalidPolicy) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list project access policies"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": policies})
}

func (s *Server) authorizeProject(w http.ResponseWriter, r *http.Request, project, repository string) bool {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "authentication required"})
		return false
	}
	if err := s.access.Check(r.Context(), identity, project, repository); err != nil {
		if errors.Is(err, access.ErrAccessDenied) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: access.ErrAccessDenied.Error()})
			return false
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to authorize project access"})
		return false
	}
	return true
}
