package repositorysync

import (
	"path"
	"regexp"
	"strings"
)

type redactionRule struct {
	pattern     *regexp.Regexp
	replacement string
}

var redactionRules = []redactionRule{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`), `[REDACTED:private_key]`},
	{regexp.MustCompile(`(?im)(\b(?:password|passwd|secret|api[_-]?key|access[_-]?token|client[_-]?secret|private[_-]?key)\b\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s#,\r\n]+)`), `${1}[REDACTED:secret]`},
	{regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+.-]*://[^:\s/@]+:)[^@\s/]+(@)`), `${1}[REDACTED:password]${2}`},
	{regexp.MustCompile(`(?i)(\bBearer\s+)[A-Za-z0-9._~+/-]+=*`), `${1}[REDACTED:bearer_token]`},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), `[REDACTED:aws_access_key]`},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,255}\b`), `[REDACTED:github_token]`},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\b`), `[REDACTED:jwt]`},
}

func redact(content string) (string, int) {
	count := 0
	for _, rule := range redactionRules {
		count += len(rule.pattern.FindAllStringIndex(content, -1))
		content = rule.pattern.ReplaceAllString(content, rule.replacement)
	}
	return content, count
}

func sensitiveFile(filePath string) bool {
	base := strings.ToLower(path.Base(filePath))
	extension := strings.ToLower(path.Ext(base))
	if base == ".env" || strings.HasPrefix(base, ".env.") || base == ".npmrc" ||
		base == ".pypirc" || base == ".netrc" || base == "credentials" ||
		strings.HasPrefix(base, "credentials.") || base == "secrets" ||
		strings.HasPrefix(base, "secrets.") || base == "id_rsa" || base == "id_ed25519" {
		return true
	}
	switch extension {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	default:
		return false
	}
}
