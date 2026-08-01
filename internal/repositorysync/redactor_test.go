package repositorysync

import (
	"strings"
	"testing"
)

func TestRedactHighConfidenceSecrets(t *testing.T) {
	content := `password = "correct-horse"
Authorization: Bearer bearer-value.123
aws = AKIA1234567890ABCDEF
token = ghp_123456789012345678901234567890
jwt = eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTYifQ.signature123
database_url = postgres://service:db-password@database.example/orders
-----BEGIN PRIVATE KEY-----
private material
-----END PRIVATE KEY-----`
	redacted, count := redact(content)
	for _, secret := range []string{"correct-horse", "bearer-value", "AKIA1234567890ABCDEF", "ghp_123456789012345678901234567890", "private material", "eyJhbGciOiJIUzI1NiJ9", "db-password"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("secret remained after redaction: %q in %s", secret, redacted)
		}
	}
	if count != 7 || !strings.Contains(redacted, "[REDACTED:private_key]") || !strings.Contains(redacted, "[REDACTED:secret]") {
		t.Fatalf("unexpected redaction result count=%d content=%s", count, redacted)
	}
}

func TestSensitiveFileDetection(t *testing.T) {
	for _, filePath := range []string{".env", ".env.production", ".npmrc", "config/secrets.enc.yaml", "credentials.toml", "certs/service.pem", "id_rsa"} {
		if !sensitiveFile(filePath) {
			t.Fatalf("expected sensitive file: %s", filePath)
		}
	}
	for _, filePath := range []string{"README.md", "config/application.yaml", "internal/key.go"} {
		if sensitiveFile(filePath) {
			t.Fatalf("unexpected sensitive file: %s", filePath)
		}
	}
}
