package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ekbda/internal/standards"
)

func (s *Server) createStandardPackage(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	var input standards.CreatePackageInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	standard, err := s.standards.CreatePackage(r.Context(), input, userID(r))
	if err != nil {
		if errors.Is(err, standards.ErrInvalidPackage) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to publish standard package"})
		return
	}
	writeJSON(w, http.StatusCreated, standard)
}

func (s *Server) getStandardPackage(w http.ResponseWriter, r *http.Request) {
	standard, err := s.standards.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, standards.ErrPackageNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load standard package"})
		return
	}
	writeJSON(w, http.StatusOK, standard)
}

func (s *Server) listStandardPackages(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	packages, err := s.standards.ListPackages(
		r.Context(), r.URL.Query().Get("name"), r.URL.Query().Get("scope"),
		r.URL.Query().Get("selector"), limit,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list standard packages"})
		return
	}
	summaries := make([]standards.PackageSummary, 0, len(packages))
	for _, standard := range packages {
		summaries = append(summaries, standard.Summary())
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": summaries})
}

func (s *Server) validateStandards(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20))
	decoder.DisallowUnknownFields()
	var input standards.ValidateInput
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON request"})
		return
	}
	if !s.authorizeProject(w, r, input.Project, "") {
		return
	}
	report, err := s.standards.Validate(r.Context(), input, userID(r))
	if err != nil {
		switch {
		case errors.Is(err, standards.ErrInvalidValidation):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		case errors.Is(err, standards.ErrNoApplicablePackages), errors.Is(err, standards.ErrApplicableRuleConflict):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate project standards"})
		}
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) getStandardReport(w http.ResponseWriter, r *http.Request) {
	report, err := s.standards.GetReport(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, standards.ErrReportNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load standard validation report"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) listStandardReports(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reports, err := s.standards.ListReports(r.Context(), r.URL.Query().Get("project"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list standard validation reports"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}
