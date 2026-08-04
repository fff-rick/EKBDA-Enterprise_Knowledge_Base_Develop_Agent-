package release

import (
	"testing"
	"time"
)

func TestWebhookVerifierAuthenticatesTimestampedPayload(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	timestamp := "1785837600"
	body := []byte(`{"event_id":"event-1"}`)
	verifier, err := NewWebhookVerifier(secret, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }
	signature := SignWebhook(secret, timestamp, body)
	if err := verifier.Verify(timestamp, signature, body); err != nil {
		t.Fatalf("valid webhook rejected: %v", err)
	}
	if err := verifier.Verify(timestamp, signature, []byte(`{"event_id":"tampered"}`)); err == nil {
		t.Fatal("tampered webhook accepted")
	}
	verifier.now = func() time.Time { return now.Add(6 * time.Minute) }
	if err := verifier.Verify(timestamp, signature, body); err == nil {
		t.Fatal("expired webhook accepted")
	}
}

func TestHTTPConnectorRequiresEnterpriseTransportAndCredentials(t *testing.T) {
	for _, config := range []ConnectorConfig{{Enabled: true, BaseURL: "http://ci.example.test", Token: "token", Timeout: time.Minute}, {Enabled: true, BaseURL: "https://ci.example.test", Timeout: time.Minute}, {Enabled: true, BaseURL: "https://ci.example.test", Token: "token", Timeout: time.Millisecond}} {
		if _, err := NewHTTPConnector(config); err == nil {
			t.Fatalf("unsafe connector config accepted: %#v", config)
		}
	}
	disabled, err := NewHTTPConnector(ConnectorConfig{})
	if err != nil || disabled.Enabled() {
		t.Fatalf("disabled connector invalid: %v", err)
	}
}
