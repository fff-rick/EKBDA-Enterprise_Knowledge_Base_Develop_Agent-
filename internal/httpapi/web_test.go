package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFrontendServesCompleteTestConsole(t *testing.T) {
	handler := (&Server{}).frontend()
	for _, target := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/", "text/html", "企业功能测试台"},
		{"/app.js", "javascript", "knowledge.documents.create"},
		{"/styles.css", "text/css", "--signal"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target.path, nil)
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", target.path, recorder.Code)
		}
		if !strings.Contains(recorder.Header().Get("Content-Type"), target.contentType) {
			t.Fatalf("GET %s content type = %q", target.path, recorder.Header().Get("Content-Type"))
		}
		if !strings.Contains(recorder.Body.String(), target.contains) {
			t.Fatalf("GET %s missing %q", target.path, target.contains)
		}
	}
}

func TestFrontendCatalogCoversEveryAPIRoute(t *testing.T) {
	serverSource, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	appSource, err := webFiles.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}

	routePattern := regexp.MustCompile(`"(?:GET|POST) (/[^" ]+)"`)
	matches := routePattern.FindAllStringSubmatch(string(serverSource), -1)
	if len(matches) == 0 {
		t.Fatal("no HTTP routes found in server.go")
	}
	for _, match := range matches {
		path := match[1]
		if path == "/" {
			continue
		}
		if !strings.Contains(string(appSource), path) {
			t.Errorf("frontend operation catalog does not cover %s", path)
		}
	}
}

func TestServerRoutesFrontendWithoutShadowingHealth(t *testing.T) {
	server := &Server{mux: http.NewServeMux()}
	server.routes()

	for _, path := range []string{"/", "/app.js", "/healthz"} {
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, recorder.Code)
		}
	}
}
