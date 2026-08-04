package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ekbda/internal/release"
)

type releaseTestConnector struct{}

func (releaseTestConnector) Enabled() bool { return true }
func (releaseTestConnector) Trigger(context.Context, release.TriggerRequest) (release.ProviderRun, error) {
	return release.ProviderRun{ID: "run-http-1", URL: "https://ci.example.test/runs/http-1"}, nil
}
func (releaseTestConnector) Rollback(context.Context, release.RollbackRequest) (release.ProviderRun, error) {
	return release.ProviderRun{ID: "rollback-http-1", URL: "https://ci.example.test/runs/http-2"}, nil
}

func TestReleaseProviderWebhookRequiresSignatureAndIsIdempotent(t *testing.T) {
	reader := release.DevelopmentReaderFunc(func(context.Context, string) (release.DevelopmentSession, error) {
		return release.DevelopmentSession{ID: "dev-http-1", Project: "order-service", Repository: "services/order", Status: "delivered", DeliveryStatus: "passed", Commit: "0123456789abcdef0123456789abcdef01234567", PullRequestURL: "https://git.example.test/acme/order/pull/8"}, nil
	})
	service, err := release.NewService(release.NewMemoryStore(), reader, releaseTestConnector{}, []string{"deploy"}, []string{"staging", "production"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := service.Create(context.Background(), release.CreateInput{DevelopmentSessionID: "dev-http-1", Environment: "staging", Pipeline: "deploy", ChangeTicket: "CHG-200", ManifestSHA256: strings.Repeat("a", 64), ConfigurationSHA256: strings.Repeat("b", 64), RollbackPlan: "restore the previously signed artifact"}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	request, _, err = service.ReconcileCodePlatform(context.Background(), release.CodePlatformEvent{EventID: "source-http-1", ReleaseID: request.ID, PullRequestURL: request.PullRequestURL, HeadCommit: request.SourceCommit, MergeCommit: "89abcdef0123456789abcdef0123456789abcdef", ProtectedBranch: true, Approvals: 2, RequiredApprovals: 2, ChecksPassed: true, Merged: true}, []byte(`{"event":"merged"}`))
	if err != nil {
		t.Fatal(err)
	}
	request, err = service.Decide(context.Background(), request.ID, release.DecisionInput{Revision: request.Revision, Decision: "approve"}, "approver-1")
	if err != nil {
		t.Fatal(err)
	}
	request, err = service.Trigger(context.Background(), request.ID, release.TriggerInput{Revision: request.Revision, Confirmation: request.TriggerConfirmation}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	checks := make([]release.CheckEvidence, 0, len(release.RequiredChecks))
	for _, name := range release.RequiredChecks {
		checks = append(checks, release.CheckEvidence{Name: name, Status: "passed", EvidenceURI: "https://evidence.example.test/" + name, SHA256: strings.Repeat("c", 64)})
	}
	event := release.ProviderEvent{EventID: "event-http-1", ReleaseID: request.ID, RunID: request.RunID, Phase: release.ProviderPhaseDeploy, Status: release.ProviderStatusSucceeded, Artifact: &release.ArtifactEvidence{URI: "https://artifacts.example.test/order/image", Digest: "sha256:" + strings.Repeat("d", 64), SBOMURI: "https://artifacts.example.test/order/sbom.json", SBOMSHA256: strings.Repeat("e", 64), SignatureVerified: true, ProvenanceVerified: true}, Checks: checks}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	secret := "0123456789abcdef0123456789abcdef"
	verifier, err := release.NewWebhookVerifier(secret, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewWithRelease(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, service, verifier)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	call := func(signature string) *httptest.ResponseRecorder {
		httpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/releases/webhooks/provider", strings.NewReader(string(body)))
		httpRequest.Header.Set("X-EKBDA-Timestamp", timestamp)
		httpRequest.Header.Set("X-EKBDA-Signature", signature)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httpRequest)
		return response
	}
	unauthorized := call("sha256=00")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned webhook status: %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	signature := release.SignWebhook(secret, timestamp, body)
	first := call(signature)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"applied":true`) {
		t.Fatalf("first webhook: %d %s", first.Code, first.Body.String())
	}
	duplicate := call(signature)
	if duplicate.Code != http.StatusOK || !strings.Contains(duplicate.Body.String(), `"applied":false`) {
		t.Fatalf("duplicate webhook: %d %s", duplicate.Code, duplicate.Body.String())
	}
}

func TestCodePlatformWebhookGatesProtectedBranchPromotion(t *testing.T) {
	reader := release.DevelopmentReaderFunc(func(context.Context, string) (release.DevelopmentSession, error) {
		return release.DevelopmentSession{ID: "dev-code-1", Project: "order-service", Repository: "services/order", Status: "delivered", DeliveryStatus: "passed", Commit: "0123456789abcdef0123456789abcdef01234567", PullRequestURL: "https://git.example.test/acme/order/pull/9"}, nil
	})
	service, err := release.NewService(release.NewMemoryStore(), reader, releaseTestConnector{}, []string{"deploy"}, []string{"staging"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := service.Create(context.Background(), release.CreateInput{DevelopmentSessionID: "dev-code-1", Environment: "staging", Pipeline: "deploy", ChangeTicket: "CHG-201", ManifestSHA256: strings.Repeat("a", 64), ConfigurationSHA256: strings.Repeat("b", 64), RollbackPlan: "restore the previously signed artifact"}, "engineer-1")
	if err != nil {
		t.Fatal(err)
	}
	event := release.CodePlatformEvent{EventID: "code-http-1", ReleaseID: value.ID, PullRequestURL: value.PullRequestURL, HeadCommit: value.SourceCommit, MergeCommit: "89abcdef0123456789abcdef0123456789abcdef", ProtectedBranch: true, Approvals: 2, RequiredApprovals: 2, ChecksPassed: true, Merged: true}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	secret := "abcdef0123456789abcdef0123456789"
	verifier, err := release.NewWebhookVerifier(secret, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewWithReleaseWebhooks(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, service, verifier, nil)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	send := func(signature string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/releases/webhooks/code-platform", strings.NewReader(string(body)))
		request.Header.Set("X-EKBDA-Timestamp", timestamp)
		request.Header.Set("X-EKBDA-Signature", signature)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	unauthorized := send("sha256=00")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned code webhook: %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	first := send(release.SignWebhook(secret, timestamp, body))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"status":"awaiting_approval"`) {
		t.Fatalf("code webhook: %d %s", first.Code, first.Body.String())
	}
	duplicate := send(release.SignWebhook(secret, timestamp, body))
	if duplicate.Code != http.StatusOK || !strings.Contains(duplicate.Body.String(), `"applied":false`) {
		t.Fatalf("duplicate code webhook: %d %s", duplicate.Code, duplicate.Body.String())
	}
}
