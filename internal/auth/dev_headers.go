package auth

import (
	"fmt"
	"net/http"
	"strings"
)

type DevHeaders struct{}

func NewDevHeaders() *DevHeaders {
	return &DevHeaders{}
}

func (*DevHeaders) Mode() string { return "dev_headers" }

func (*DevHeaders) Authenticate(request *http.Request) (Identity, error) {
	userID := strings.TrimSpace(request.Header.Get("X-User-ID"))
	if userID == "" {
		return Identity{}, fmt.Errorf("%w: X-User-ID header is required", ErrUnauthenticated)
	}
	return Identity{
		UserID: userID,
		Roles:  normalizeRoles(strings.Split(request.Header.Get("X-User-Roles"), ",")),
		Source: "dev_headers",
	}, nil
}

func normalizeRoles(values []string) []string {
	roles := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		roles = append(roles, value)
	}
	return roles
}
