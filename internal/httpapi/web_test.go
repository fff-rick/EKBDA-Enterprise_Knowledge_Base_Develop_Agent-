package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrontendServesLandingPage(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	(&Server{}).frontend().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("frontend response status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "企业知识库开发助手") {
		t.Fatal("frontend response does not contain the landing page")
	}
}
