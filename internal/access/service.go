package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ekbda/internal/auth"
)

const (
	maxMembers      = 500
	defaultListSize = 100
	maxListSize     = 500
)

var (
	projectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	rolePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._:/-]{0,127}$`)
)

type Service struct {
	store Store
	mode  string
}

func New(store Store, mode string) (*Service, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = ModeDisabled
	}
	if store == nil || (mode != ModeDisabled && mode != ModeEnforced) {
		return nil, fmt.Errorf("invalid project authorization configuration")
	}
	return &Service{store: store, mode: mode}, nil
}

func (s *Service) Mode() string { return s.mode }

func (s *Service) CreatePolicy(ctx context.Context, input CreatePolicyInput, createdBy string) (Policy, error) {
	policy, err := normalizePolicy(input, createdBy)
	if err != nil {
		return Policy{}, err
	}
	return s.store.CreatePolicy(ctx, policy)
}

func (s *Service) GetLatest(ctx context.Context, project string) (Policy, error) {
	project, err := normalizeProject(project)
	if err != nil {
		return Policy{}, ErrInvalidPolicy
	}
	return s.store.GetLatest(ctx, project)
}

func (s *Service) ListPolicies(ctx context.Context, project string, limit int) ([]Policy, error) {
	project, err := normalizeProject(project)
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	if limit < 1 {
		limit = defaultListSize
	}
	if limit > maxListSize {
		limit = maxListSize
	}
	return s.store.ListPolicies(ctx, project, limit)
}

// Check authorizes a trusted identity for a project and, when supplied, an
// exact repository path. Governance administrators always retain access.
func (s *Service) Check(ctx context.Context, identity auth.Identity, project, repository string) error {
	if hasRole(identity.Roles, "knowledge_admin") || s.mode == ModeDisabled {
		return nil
	}
	project, err := normalizeProject(project)
	if err != nil || strings.TrimSpace(identity.UserID) == "" {
		return ErrAccessDenied
	}
	policy, err := s.store.GetLatest(ctx, project)
	if errors.Is(err, ErrPolicyNotFound) {
		return ErrAccessDenied
	}
	if err != nil {
		return err
	}
	if !contains(policy.Users, identity.UserID) && !rolesIntersect(policy.Roles, identity.Roles) {
		return ErrAccessDenied
	}
	if repository == "" {
		return nil
	}
	repository, err = normalizeRepository(repository)
	if err != nil || !contains(policy.Repositories, repository) {
		return ErrAccessDenied
	}
	return nil
}

func normalizePolicy(input CreatePolicyInput, createdBy string) (Policy, error) {
	project, err := normalizeProject(input.Project)
	if err != nil || strings.TrimSpace(input.Owner) == "" || len(input.Owner) > 256 ||
		len(input.Description) > 2000 || strings.TrimSpace(createdBy) == "" || len(createdBy) > 256 ||
		len(input.Users) > maxMembers || len(input.Roles) > maxMembers || len(input.Repositories) > maxMembers {
		return Policy{}, ErrInvalidPolicy
	}
	users, err := normalizeValues(input.Users, false, nil)
	if err != nil {
		return Policy{}, ErrInvalidPolicy
	}
	roles, err := normalizeValues(input.Roles, true, rolePattern)
	if err != nil {
		return Policy{}, ErrInvalidPolicy
	}
	repositories := make([]string, 0, len(input.Repositories))
	seenRepositories := make(map[string]struct{}, len(input.Repositories))
	for _, repository := range input.Repositories {
		normalized, err := normalizeRepository(repository)
		if err != nil {
			return Policy{}, ErrInvalidPolicy
		}
		if _, exists := seenRepositories[normalized]; !exists {
			seenRepositories[normalized] = struct{}{}
			repositories = append(repositories, normalized)
		}
	}
	sort.Strings(repositories)
	definition := struct {
		Project, Description, Owner string
		Users, Roles, Repositories  []string
	}{project, strings.TrimSpace(input.Description), strings.TrimSpace(input.Owner), users, roles, repositories}
	encoded, err := json.Marshal(definition)
	if err != nil {
		return Policy{}, fmt.Errorf("encode project access policy: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return Policy{
		ID: newID(), Project: project, Description: definition.Description, Owner: definition.Owner,
		DefinitionHash: hex.EncodeToString(digest[:]), Users: users, Roles: roles,
		Repositories: repositories, CreatedBy: strings.TrimSpace(createdBy), CreatedAt: time.Now().UTC(),
	}, nil
}

func normalizeProject(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !projectPattern.MatchString(value) {
		return "", ErrInvalidPolicy
	}
	return value, nil
}

func normalizeRepository(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") {
		return "", ErrInvalidPolicy
	}
	firstPart := strings.SplitN(value, "/", 2)[0]
	if strings.Contains(firstPart, ":") {
		return "", ErrInvalidPolicy
	}
	normalized := path.Clean(value)
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", ErrInvalidPolicy
	}
	return normalized, nil
}

func normalizeValues(values []string, lower bool, pattern *regexp.Regexp) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n\x00") || (pattern != nil && !pattern.MatchString(value)) {
			return nil, ErrInvalidPolicy
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), target) {
			return true
		}
	}
	return false
}

func rolesIntersect(policyRoles, identityRoles []string) bool {
	for _, identityRole := range identityRoles {
		identityRole = strings.ToLower(strings.TrimSpace(identityRole))
		if contains(policyRoles, identityRole) {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("access-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
