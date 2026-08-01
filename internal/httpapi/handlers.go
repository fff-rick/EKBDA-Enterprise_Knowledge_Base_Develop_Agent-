package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ekbda/internal/answer"
	"ekbda/internal/auth"
	"ekbda/internal/evaluation"
	"ekbda/internal/ingestion"
	"ekbda/internal/knowledge"
)

type errorResponse struct {
	Error   string `json:"error"`
	TraceID string `json:"trace_id,omitempty"`
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createDocument(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	var input knowledge.CreateDocumentInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	if !s.authorizeProject(w, r, input.Project, "") {
		return
	}
	document, err := s.knowledge.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, knowledge.ErrInvalidDocument) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "title, content, source_uri, project and a valid classification are required; restricted documents also require allowed_roles"})
			return
		}
		if errors.Is(err, knowledge.ErrDocumentExists) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to store document"})
		return
	}
	writeJSON(w, http.StatusCreated, document)
}

func (s *Server) createImport(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input ingestion.Input
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	if !s.authorizeProject(w, r, input.Project, "") {
		return
	}
	report, err := s.ingestion.Start(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ingestion.ErrDisabled):
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
		case errors.Is(err, ingestion.ErrInvalidPath):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusAccepted, report)
}

func (s *Server) getImport(w http.ResponseWriter, r *http.Request) {
	report, err := s.ingestion.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ingestion.ErrJobNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load ingestion job"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) listDocumentVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.knowledge.Versions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load document versions"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	project := r.URL.Query().Get("project")
	if !s.authorizeProject(w, r, project, "") {
		return
	}
	results, err := s.knowledge.Search(r.Context(), knowledge.SearchInput{
		Query:   r.URL.Query().Get("q"),
		Project: project,
		Roles:   roles(r),
		Limit:   limit,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) createAnswer(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input answer.Input
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	if !s.authorizeProject(w, r, input.Project, "") {
		return
	}
	input.Roles = roles(r)
	input.UserID = userID(r)
	response, err := s.answer.Ask(r.Context(), input)
	if err != nil {
		if errors.Is(err, answer.ErrInvalidInput) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), TraceID: answer.ErrorTraceID(err)})
			return
		}
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to generate grounded answer", TraceID: answer.ErrorTraceID(err)})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) getAnswerTrace(w http.ResponseWriter, r *http.Request) {
	trace, err := s.answer.Trace(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, answer.ErrTraceNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load answer trace"})
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func (s *Server) getAnswerMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.answer.Metrics(r.Context(), r.URL.Query().Get("project"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to aggregate answer metrics"})
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) pruneAnswerTraces(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	deleted, before, err := s.answer.PruneTraces(r.Context(), input.RetentionDays)
	if err != nil {
		if errors.Is(err, answer.ErrInvalidRetention) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to prune answer traces"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "before": before})
}

func (s *Server) runAnswerEvaluation(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	var request evaluation.Request
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	request.UserID = userID(r)
	report, err := s.evaluation.RunAdHoc(r.Context(), request)
	if err != nil {
		if errors.Is(err, evaluation.ErrInvalidSuite) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to run answer evaluation"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) createEvaluationSuite(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	var input evaluation.CreateSuiteInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	suite, err := s.evaluation.CreateSuite(r.Context(), input, userID(r))
	if err != nil {
		if errors.Is(err, evaluation.ErrInvalidSuiteInput) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create evaluation suite"})
		return
	}
	writeJSON(w, http.StatusCreated, suite)
}

func (s *Server) getEvaluationSuite(w http.ResponseWriter, r *http.Request) {
	suite, err := s.evaluation.GetSuite(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, evaluation.ErrSuiteNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load evaluation suite"})
		return
	}
	writeJSON(w, http.StatusOK, suite)
}

func (s *Server) listEvaluationSuites(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	suites, err := s.evaluation.ListSuites(r.Context(), r.URL.Query().Get("name"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list evaluation suites"})
		return
	}
	summaries := make([]evaluation.SuiteSummary, 0, len(suites))
	for _, suite := range suites {
		summaries = append(summaries, suite.Summary())
	}
	writeJSON(w, http.StatusOK, map[string]any{"suites": summaries})
}

func (s *Server) createEvaluationRun(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input evaluation.StartRunInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	run, err := s.evaluation.StartRun(r.Context(), input, userID(r))
	if err != nil {
		if errors.Is(err, evaluation.ErrSuiteNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to start evaluation run"})
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) getEvaluationRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.evaluation.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, evaluation.ErrRunNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load evaluation run"})
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) listEvaluationRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.evaluation.ListRuns(r.Context(), r.URL.Query().Get("suite_id"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list evaluation runs"})
		return
	}
	summaries := make([]evaluation.RunSummary, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, run.Summary())
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": summaries})
}

func (s *Server) cancelEvaluationRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.evaluation.CancelRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, evaluation.ErrRunNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to cancel evaluation run"})
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) retryEvaluationRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.evaluation.RetryRun(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		switch {
		case errors.Is(err, evaluation.ErrRunNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		case errors.Is(err, evaluation.ErrRunNotRetryable):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case errors.Is(err, evaluation.ErrRunAlreadyRetried):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to retry evaluation run"})
		}
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) requireIdentity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, err := s.auth.Authenticate(r)
		if err != nil {
			if s.auth.Mode() == "jwt" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="ekbda"`)
			}
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "authentication required"})
			return
		}
		next(w, r.WithContext(auth.WithIdentity(r.Context(), identity)))
	}
}

func (s *Server) requireRole(required string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, role := range roles(r) {
			if role == required {
				next(w, r)
				return
			}
		}
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "required role: " + required})
	}
}

func roles(r *http.Request) []string {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		return []string{}
	}
	return append([]string(nil), identity.Roles...)
}

func userID(r *http.Request) string {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		return ""
	}
	return identity.UserID
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
