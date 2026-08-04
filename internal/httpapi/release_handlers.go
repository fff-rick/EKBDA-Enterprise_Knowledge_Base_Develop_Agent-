package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ekbda/internal/development"
	"ekbda/internal/release"
)

func (s *Server) getReleaseCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.releases.Catalog())
}

func (s *Server) createRelease(w http.ResponseWriter, r *http.Request) {
	if !releaseRole(r, "release_engineer") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: release_engineer"})
		return
	}
	var input release.CreateInput
	if !decodeReleaseJSON(w, r, &input) {
		return
	}
	session, err := s.development.Get(r.Context(), input.DevelopmentSessionID)
	if err != nil {
		s.writeReleaseError(w, err, "failed to load development session")
		return
	}
	if !s.authorizeProject(w, r, session.Project, session.Repository) {
		return
	}
	value, err := s.releases.Create(r.Context(), input, userID(r))
	if err != nil {
		s.writeReleaseError(w, err, "failed to create release request")
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	if project == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "project is required"})
		return
	}
	if !s.authorizeProject(w, r, project, "") {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	values, err := s.releases.List(r.Context(), project, limit)
	if err != nil {
		s.writeReleaseError(w, err, "failed to list releases")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": values})
}

func (s *Server) getRelease(w http.ResponseWriter, r *http.Request) {
	value, ok := s.authorizedRelease(w, r)
	if ok {
		writeJSON(w, http.StatusOK, value)
	}
}
func (s *Server) listReleaseEvents(w http.ResponseWriter, r *http.Request) {
	value, ok := s.authorizedRelease(w, r)
	if !ok {
		return
	}
	events, err := s.releases.Events(r.Context(), value.ID)
	if err != nil {
		s.writeReleaseError(w, err, "failed to list release events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) decideRelease(w http.ResponseWriter, r *http.Request) {
	if !releaseRole(r, "release_approver") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: release_approver"})
		return
	}
	value, ok := s.authorizedRelease(w, r)
	if !ok {
		return
	}
	var input release.DecisionInput
	if !decodeReleaseJSON(w, r, &input) {
		return
	}
	updated, err := s.releases.Decide(r.Context(), value.ID, input, userID(r))
	if err != nil {
		s.writeReleaseError(w, err, "failed to decide release")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) triggerRelease(w http.ResponseWriter, r *http.Request) {
	if !releaseRole(r, "release_engineer") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: release_engineer"})
		return
	}
	value, ok := s.authorizedRelease(w, r)
	if !ok {
		return
	}
	var input release.TriggerInput
	if !decodeReleaseJSON(w, r, &input) {
		return
	}
	updated, err := s.releases.Trigger(r.Context(), value.ID, input, userID(r))
	if err != nil {
		s.writeReleaseError(w, err, "failed to trigger release")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) requestReleaseRollback(w http.ResponseWriter, r *http.Request) {
	if !releaseRole(r, "release_engineer") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: release_engineer"})
		return
	}
	value, ok := s.authorizedRelease(w, r)
	if !ok {
		return
	}
	var input release.RollbackInput
	if !decodeReleaseJSON(w, r, &input) {
		return
	}
	updated, err := s.releases.RequestRollback(r.Context(), value.ID, input, userID(r))
	if err != nil {
		s.writeReleaseError(w, err, "failed to request rollback")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) decideReleaseRollback(w http.ResponseWriter, r *http.Request) {
	if !releaseRole(r, "release_approver") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: release_approver"})
		return
	}
	value, ok := s.authorizedRelease(w, r)
	if !ok {
		return
	}
	var input release.DecisionInput
	if !decodeReleaseJSON(w, r, &input) {
		return
	}
	updated, err := s.releases.DecideRollback(r.Context(), value.ID, input, userID(r))
	if err != nil {
		s.writeReleaseError(w, err, "failed to decide rollback")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) triggerReleaseRollback(w http.ResponseWriter, r *http.Request) {
	if !releaseRole(r, "release_engineer") {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: release_engineer"})
		return
	}
	value, ok := s.authorizedRelease(w, r)
	if !ok {
		return
	}
	var input release.TriggerInput
	if !decodeReleaseJSON(w, r, &input) {
		return
	}
	updated, err := s.releases.TriggerRollback(r.Context(), value.ID, input, userID(r))
	if err != nil {
		s.writeReleaseError(w, err, "failed to trigger rollback")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) releaseProviderWebhook(w http.ResponseWriter, r *http.Request) {
	if s.releaseWebhook == nil || !s.releases.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: release.ErrDisabled.Error()})
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook body"})
		return
	}
	if err := s.releaseWebhook.Verify(r.Header.Get("X-EKBDA-Timestamp"), r.Header.Get("X-EKBDA-Signature"), body); err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid webhook signature"})
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var event release.ProviderEvent
	if err := decoder.Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook payload"})
		return
	}
	value, applied, err := s.releases.Reconcile(r.Context(), event, body)
	if err != nil {
		s.writeReleaseError(w, err, "failed to reconcile provider event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "release": value})
}

func (s *Server) releaseCodePlatformWebhook(w http.ResponseWriter, r *http.Request) {
	if s.codeWebhook == nil || !s.releases.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: release.ErrDisabled.Error()})
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook body"})
		return
	}
	if err := s.codeWebhook.Verify(r.Header.Get("X-EKBDA-Timestamp"), r.Header.Get("X-EKBDA-Signature"), body); err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid webhook signature"})
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var event release.CodePlatformEvent
	if err := decoder.Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook payload"})
		return
	}
	value, applied, err := s.releases.ReconcileCodePlatform(r.Context(), event, body)
	if err != nil {
		s.writeReleaseError(w, err, "failed to reconcile code platform event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "release": value})
}

func (s *Server) authorizedRelease(w http.ResponseWriter, r *http.Request) (release.Request, bool) {
	value, err := s.releases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeReleaseError(w, err, "failed to load release")
		return release.Request{}, false
	}
	if !s.authorizeProject(w, r, value.Project, value.Repository) {
		return release.Request{}, false
	}
	return value, true
}

func decodeReleaseJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return false
	}
	return true
}
func releaseRole(r *http.Request, role string) bool {
	return requestHasRole(r, role) || requestHasRole(r, "knowledge_admin")
}

func (s *Server) writeReleaseError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, release.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, release.ErrNotFound), errors.Is(err, development.ErrSessionNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, release.ErrSelfApproval), errors.Is(err, release.ErrSelfRollbackApproval):
		writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error()})
	case errors.Is(err, release.ErrRevisionConflict), errors.Is(err, release.ErrInvalidTransition), errors.Is(err, release.ErrDevelopmentNotReady), errors.Is(err, release.ErrConfirmation), errors.Is(err, release.ErrProviderConflict), errors.Is(err, release.ErrProviderEvidence):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, release.ErrDisabled):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	case errors.Is(err, release.ErrProviderRejected):
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fallback})
	}
}
