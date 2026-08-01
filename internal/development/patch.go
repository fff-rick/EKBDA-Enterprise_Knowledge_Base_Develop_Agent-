package development

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxPatchBytes   = 512 << 10
	maxPatchFiles   = 50
	maxChangedLines = 10000
)

var (
	safePathPattern = regexp.MustCompile(`^[A-Za-z0-9._@/+\-]+$`)
	hunkPattern     = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(?: .*)?$`)
	indexPattern    = regexp.MustCompile(`^index [0-9a-f]{7,64}\.\.[0-9a-f]{7,64}(?: 100644)?$`)
	secretPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,255}\b`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\b`),
		regexp.MustCompile(`(?i)\b(?:password|passwd|secret|api[_-]?key|access[_-]?token|client[_-]?secret|private[_-]?key)\b\s*[:=]\s*["']?[A-Za-z0-9/+_.-]{8,}`),
	}
)

func normalizeAllowedPaths(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 50 {
		return nil, ErrInvalidInput
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
		value = strings.TrimSuffix(value, "/")
		if !validRepositoryPath(value) || value == "." {
			return nil, ErrInvalidInput
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, ErrInvalidInput
	}
	return result, nil
}

func validateAllowedCommands(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > len(commandCatalog) {
		return nil, ErrInvalidInput
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, found := commandCatalog[value]; !found {
			return nil, ErrInvalidInput
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validateProposalPatch(raw string, allowedPaths []string) (string, []FileChange, error) {
	if raw == "" || len(raw) > maxPatchBytes || !utf8.ValidString(raw) || strings.IndexByte(raw, 0) >= 0 || !strings.HasSuffix(raw, "\n") {
		return "", nil, ErrInvalidPatch
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.Contains(normalized, "\r") {
		return "", nil, ErrInvalidPatch
	}
	lines := strings.Split(normalized, "\n")
	changes := make([]FileChange, 0)
	seen := make(map[string]struct{})
	for index := 0; index < len(lines)-1; {
		if lines[index] == "" {
			index++
			continue
		}
		if !strings.HasPrefix(lines[index], "diff --git a/") {
			return "", nil, ErrInvalidPatch
		}
		fields := strings.Fields(lines[index])
		if len(fields) != 4 || !strings.HasPrefix(fields[2], "a/") || !strings.HasPrefix(fields[3], "b/") {
			return "", nil, ErrInvalidPatch
		}
		filePath := strings.TrimPrefix(fields[2], "a/")
		if filePath != strings.TrimPrefix(fields[3], "b/") || !validRepositoryPath(filePath) {
			return "", nil, ErrInvalidPatch
		}
		if sensitivePath(filePath) {
			return "", nil, ErrSensitiveContent
		}
		if !pathAllowed(filePath, allowedPaths) {
			return "", nil, ErrPathNotAllowed
		}
		if _, found := seen[filePath]; found {
			return "", nil, ErrInvalidPatch
		}
		seen[filePath] = struct{}{}
		index++
		operation := "modify"
		oldHeader, newHeader, hunks := false, false, 0
		change := FileChange{Path: filePath, Operation: operation}
		for index < len(lines)-1 && !strings.HasPrefix(lines[index], "diff --git ") {
			line := lines[index]
			switch {
			case line == "new file mode 100644":
				if oldHeader || newHeader || hunks > 0 || operation != "modify" {
					return "", nil, ErrInvalidPatch
				}
				operation = "add"
			case line == "deleted file mode 100644":
				if oldHeader || newHeader || hunks > 0 || operation != "modify" {
					return "", nil, ErrInvalidPatch
				}
				operation = "delete"
			case strings.HasPrefix(line, "index "):
				if oldHeader || newHeader || hunks > 0 || !indexPattern.MatchString(line) {
					return "", nil, ErrInvalidPatch
				}
			case strings.HasPrefix(line, "--- "):
				if oldHeader || newHeader || hunks > 0 {
					return "", nil, ErrInvalidPatch
				}
				expected := "a/" + filePath
				if operation == "add" {
					expected = "/dev/null"
				}
				if line != "--- "+expected {
					return "", nil, ErrInvalidPatch
				}
				oldHeader = true
			case strings.HasPrefix(line, "+++ "):
				if newHeader || hunks > 0 {
					return "", nil, ErrInvalidPatch
				}
				expected := "b/" + filePath
				if operation == "delete" {
					expected = "/dev/null"
				}
				if line != "+++ "+expected || !oldHeader {
					return "", nil, ErrInvalidPatch
				}
				newHeader = true
			case strings.HasPrefix(line, "@@ "):
				if !newHeader {
					return "", nil, ErrInvalidPatch
				}
				match := hunkPattern.FindStringSubmatch(line)
				if match == nil {
					return "", nil, ErrInvalidPatch
				}
				oldExpected := hunkCount(match[2])
				newExpected := hunkCount(match[4])
				oldActual, newActual := 0, 0
				hunks++
				index++
				for index < len(lines)-1 && !strings.HasPrefix(lines[index], "@@ ") && !strings.HasPrefix(lines[index], "diff --git ") {
					content := lines[index]
					if content == `\ No newline at end of file` {
						index++
						continue
					}
					if content == "" {
						return "", nil, ErrInvalidPatch
					}
					switch content[0] {
					case ' ':
						oldActual++
						newActual++
					case '-':
						oldActual++
						change.Deletions++
					case '+':
						newActual++
						change.Additions++
						if suspectedSecret(content[1:]) {
							return "", nil, ErrSensitiveContent
						}
					default:
						return "", nil, ErrInvalidPatch
					}
					index++
				}
				if oldActual != oldExpected || newActual != newExpected {
					return "", nil, ErrInvalidPatch
				}
				continue
			case line == "":
			default:
				return "", nil, ErrInvalidPatch
			}
			index++
		}
		change.Operation = operation
		if !oldHeader || !newHeader || hunks == 0 || change.Additions+change.Deletions == 0 {
			return "", nil, ErrInvalidPatch
		}
		changes = append(changes, change)
		if len(changes) > maxPatchFiles || totalChangedLines(changes) > maxChangedLines {
			return "", nil, ErrInvalidPatch
		}
	}
	if len(changes) == 0 {
		return "", nil, ErrInvalidPatch
	}
	return normalized, changes, nil
}

func hunkCount(value string) int {
	if value == "" {
		return 1
	}
	count, _ := strconv.Atoi(value)
	return count
}

func totalChangedLines(changes []FileChange) int {
	total := 0
	for _, change := range changes {
		total += change.Additions + change.Deletions
	}
	return total
}

func validRepositoryPath(value string) bool {
	if value == "" || len(value) > 300 || !safePathPattern.MatchString(value) || strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func pathAllowed(filePath string, allowed []string) bool {
	for _, prefix := range allowed {
		if filePath == prefix || strings.HasPrefix(filePath, prefix+"/") {
			return true
		}
	}
	return false
}

func sensitivePath(filePath string) bool {
	base := strings.ToLower(path.Base(filePath))
	extension := strings.ToLower(path.Ext(base))
	if filePath == ".git" || strings.HasPrefix(filePath, ".git/") || base == ".env" || strings.HasPrefix(base, ".env.") || base == ".npmrc" || base == ".pypirc" || base == ".netrc" || base == "credentials" || strings.HasPrefix(base, "credentials.") || base == "secrets" || strings.HasPrefix(base, "secrets.") || base == "id_rsa" || base == "id_ed25519" {
		return true
	}
	switch extension {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	default:
		return false
	}
}

func suspectedSecret(value string) bool {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func patchHash(baseline, normalizedPatch string, commandIDs []string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s", baseline, strings.Join(commandIDs, ","), normalizedPatch)))
	return hex.EncodeToString(digest[:])
}
