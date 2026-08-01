package standards

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
)

var (
	ErrInvalidPackage         = errors.New("standard package definition is invalid")
	ErrInvalidValidation      = errors.New("project, technology and 1 to 1000 valid files are required")
	ErrNoApplicablePackages   = errors.New("no standard packages apply to this project and technology")
	ErrApplicableRuleConflict = errors.New("applicable standard packages contain duplicate rule IDs")
)

const (
	maxPackageRules      = 200
	maxValidationFiles   = 1000
	maxFileContentBytes  = 1 << 20
	maxTotalContentBytes = 5 << 20
)

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreatePackage(ctx context.Context, input CreatePackageInput, createdBy string) (Package, error) {
	input.Rules = cloneRules(input.Rules)
	normalizePackageInput(&input)
	if err := validatePackageInput(input); err != nil {
		return Package{}, err
	}
	definition, err := json.Marshal(struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Scope       string `json:"scope"`
		Selector    string `json:"selector"`
		Owner       string `json:"owner"`
		Rules       []Rule `json:"rules"`
	}{Name: input.Name, Description: input.Description, Scope: input.Scope, Selector: input.Selector, Owner: input.Owner, Rules: input.Rules})
	if err != nil {
		return Package{}, fmt.Errorf("encode standard package definition: %w", err)
	}
	hash := sha256.Sum256(definition)
	return s.store.CreatePackage(ctx, Package{
		ID: newID(), Name: input.Name, Description: input.Description,
		Scope: input.Scope, Selector: input.Selector, Owner: input.Owner,
		DefinitionHash: hex.EncodeToString(hash[:]), Rules: cloneRules(input.Rules),
		CreatedBy: strings.TrimSpace(createdBy), CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) GetPackage(ctx context.Context, id string) (Package, error) {
	return s.store.GetPackage(ctx, strings.TrimSpace(id))
}

func (s *Service) ListPackages(ctx context.Context, name, scope, selector string, limit int) ([]Package, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	scope = strings.ToLower(strings.TrimSpace(scope))
	selector = strings.ToLower(strings.TrimSpace(selector))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.ListPackages(ctx, name, scope, selector, limit)
}

func (s *Service) ApplicablePackages(ctx context.Context, project, technology string) ([]Package, error) {
	project = strings.ToLower(strings.TrimSpace(project))
	technology = strings.ToLower(strings.TrimSpace(technology))
	if !keyPattern.MatchString(project) || !keyPattern.MatchString(technology) {
		return nil, ErrInvalidValidation
	}
	return s.store.ListApplicable(ctx, project, technology)
}

func (s *Service) Validate(ctx context.Context, input ValidateInput, validatedBy string) (ValidationReport, error) {
	input.Project = strings.ToLower(strings.TrimSpace(input.Project))
	input.Technology = strings.ToLower(strings.TrimSpace(input.Technology))
	files, err := normalizeFiles(input.Files)
	if err != nil || !keyPattern.MatchString(input.Project) || !keyPattern.MatchString(input.Technology) {
		return ValidationReport{}, ErrInvalidValidation
	}
	packages, err := s.store.ListApplicable(ctx, input.Project, input.Technology)
	if err != nil {
		return ValidationReport{}, err
	}
	if len(packages) == 0 {
		return ValidationReport{}, ErrNoApplicablePackages
	}
	seenRules := make(map[string]string)
	for _, standard := range packages {
		if err := validatePackageInput(CreatePackageInput{
			Name: standard.Name, Description: standard.Description, Scope: standard.Scope,
			Selector: standard.Selector, Owner: standard.Owner, Rules: standard.Rules,
		}); err != nil {
			return ValidationReport{}, fmt.Errorf("stored standard package %s is invalid", standard.ID)
		}
		for _, rule := range standard.Rules {
			if packageName, exists := seenRules[rule.ID]; exists {
				return ValidationReport{}, fmt.Errorf("%w: %s exists in %s and %s", ErrApplicableRuleConflict, rule.ID, packageName, standard.Name)
			}
			seenRules[rule.ID] = standard.Name
		}
	}
	inputHash, err := hashFiles(files)
	if err != nil {
		return ValidationReport{}, fmt.Errorf("hash standard validation input: %w", err)
	}
	report := ValidationReport{
		ID: newID(), Project: input.Project, Technology: input.Technology,
		InputHash: inputHash, Passed: true, ValidatedBy: strings.TrimSpace(validatedBy),
		CreatedAt: time.Now().UTC(), Packages: make([]PackageReference, 0, len(packages)),
		Violations: []Violation{},
	}
	for _, standard := range packages {
		report.Packages = append(report.Packages, PackageReference{
			ID: standard.ID, Name: standard.Name, Scope: standard.Scope,
			Selector: standard.Selector, Version: standard.Version, DefinitionHash: standard.DefinitionHash,
		})
		for _, rule := range standard.Rules {
			report.RuleCount++
			violations := evaluateRule(standard, rule, files)
			for _, violation := range violations {
				if violation.Level == LevelBlock {
					report.BlockingCount++
				}
				report.Violations = append(report.Violations, violation)
			}
		}
	}
	report.ViolationCount = len(report.Violations)
	report.Passed = report.BlockingCount == 0
	if err := s.store.SaveReport(ctx, report); err != nil {
		return ValidationReport{}, fmt.Errorf("save standard validation report: %w", err)
	}
	return report, nil
}

func (s *Service) GetReport(ctx context.Context, id string) (ValidationReport, error) {
	return s.store.GetReport(ctx, strings.TrimSpace(id))
}

func (s *Service) ListReports(ctx context.Context, project string, limit int) ([]ValidationReport, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.store.ListReports(ctx, strings.ToLower(strings.TrimSpace(project)), limit)
}

func normalizePackageInput(input *CreatePackageInput) {
	input.Name = strings.ToLower(strings.TrimSpace(input.Name))
	input.Description = strings.TrimSpace(input.Description)
	input.Scope = strings.ToLower(strings.TrimSpace(input.Scope))
	input.Selector = strings.ToLower(strings.TrimSpace(input.Selector))
	input.Owner = strings.TrimSpace(input.Owner)
	for index := range input.Rules {
		input.Rules[index].ID = strings.ToLower(strings.TrimSpace(input.Rules[index].ID))
		input.Rules[index].Title = strings.TrimSpace(input.Rules[index].Title)
		input.Rules[index].Description = strings.TrimSpace(input.Rules[index].Description)
		input.Rules[index].Owner = strings.TrimSpace(input.Rules[index].Owner)
		if input.Rules[index].Owner == "" {
			input.Rules[index].Owner = input.Owner
		}
		input.Rules[index].Category = strings.ToLower(strings.TrimSpace(input.Rules[index].Category))
		input.Rules[index].Level = strings.ToLower(strings.TrimSpace(input.Rules[index].Level))
		if input.Rules[index].Check != nil {
			input.Rules[index].Check.Type = strings.ToLower(strings.TrimSpace(input.Rules[index].Check.Type))
			input.Rules[index].Check.Target = strings.TrimSpace(input.Rules[index].Check.Target)
			input.Rules[index].Check.Pattern = strings.TrimSpace(input.Rules[index].Check.Pattern)
		}
	}
}

func validatePackageInput(input CreatePackageInput) error {
	if !keyPattern.MatchString(input.Name) || input.Owner == "" || len(input.Owner) > 256 || len(input.Description) > 4000 || len(input.Rules) < 1 || len(input.Rules) > maxPackageRules {
		return ErrInvalidPackage
	}
	if input.Scope != ScopeCommon && input.Scope != ScopeTechnology && input.Scope != ScopeProject {
		return ErrInvalidPackage
	}
	if (input.Scope == ScopeCommon && input.Selector != "") || (input.Scope != ScopeCommon && !keyPattern.MatchString(input.Selector)) {
		return ErrInvalidPackage
	}
	seen := make(map[string]struct{}, len(input.Rules))
	for _, rule := range input.Rules {
		if !keyPattern.MatchString(rule.ID) || rule.Title == "" || len(rule.Title) > 200 || len(rule.Description) > 2000 || rule.Owner == "" || len(rule.Owner) > 256 {
			return ErrInvalidPackage
		}
		if _, exists := seen[rule.ID]; exists {
			return ErrInvalidPackage
		}
		seen[rule.ID] = struct{}{}
		if !validCategory(rule.Category) || !validLevel(rule.Level) {
			return ErrInvalidPackage
		}
		if rule.Level == LevelGuidance || rule.Level == LevelTemplate {
			if rule.Check != nil {
				return ErrInvalidPackage
			}
			continue
		}
		if rule.Check == nil || validateRuleCheck(*rule.Check) != nil {
			return ErrInvalidPackage
		}
	}
	return nil
}

func validateRuleCheck(check RuleCheck) error {
	if len(check.Target) > 512 || len(check.Pattern) > 512 {
		return ErrInvalidPackage
	}
	switch check.Type {
	case CheckRequiredPath:
		normalized, err := normalizePath(check.Pattern)
		if err != nil || normalized != check.Pattern || check.Target != "" || check.Minimum != 0 {
			return ErrInvalidPackage
		}
	case CheckForbiddenPath:
		if check.Target != "" || check.Minimum != 0 || compilePattern(check.Pattern) == nil {
			return ErrInvalidPackage
		}
	case CheckPathPattern, CheckContent:
		if check.Minimum != 0 || compilePattern(check.Target) == nil || compilePattern(check.Pattern) == nil {
			return ErrInvalidPackage
		}
	case CheckMinimumMatch:
		if check.Pattern != "" || check.Minimum < 1 || check.Minimum > maxValidationFiles || compilePattern(check.Target) == nil {
			return ErrInvalidPackage
		}
	default:
		return ErrInvalidPackage
	}
	return nil
}

func evaluateRule(standard Package, rule Rule, files []File) []Violation {
	if rule.Check == nil {
		return nil
	}
	newViolation := func(filePath, message string) Violation {
		return Violation{
			RuleID: rule.ID, RuleTitle: rule.Title, Category: rule.Category, Level: rule.Level,
			PackageID: standard.ID, PackageName: standard.Name, PackageVersion: standard.Version,
			Path: filePath, Message: message,
		}
	}
	check := *rule.Check
	switch check.Type {
	case CheckRequiredPath:
		for _, file := range files {
			if file.Path == check.Pattern {
				return nil
			}
		}
		return []Violation{newViolation(check.Pattern, "required path is missing")}
	case CheckForbiddenPath:
		pattern := regexp.MustCompile(check.Pattern)
		result := make([]Violation, 0)
		for _, file := range files {
			if pattern.MatchString(file.Path) {
				result = append(result, newViolation(file.Path, "forbidden path is present"))
			}
		}
		return result
	case CheckPathPattern:
		target := regexp.MustCompile(check.Target)
		pattern := regexp.MustCompile(check.Pattern)
		result := make([]Violation, 0)
		for _, file := range files {
			if target.MatchString(file.Path) && !pattern.MatchString(file.Path) {
				result = append(result, newViolation(file.Path, "path does not match the required naming pattern"))
			}
		}
		return result
	case CheckContent:
		target := regexp.MustCompile(check.Target)
		pattern := regexp.MustCompile(check.Pattern)
		result := make([]Violation, 0)
		for _, file := range files {
			if target.MatchString(file.Path) && !pattern.MatchString(file.Content) {
				result = append(result, newViolation(file.Path, "file content does not contain the required pattern"))
			}
		}
		return result
	case CheckMinimumMatch:
		target := regexp.MustCompile(check.Target)
		count := 0
		for _, file := range files {
			if target.MatchString(file.Path) {
				count++
			}
		}
		if count < check.Minimum {
			return []Violation{newViolation("", fmt.Sprintf("matched %d paths; at least %d are required", count, check.Minimum))}
		}
	}
	return nil
}

func normalizeFiles(input []File) ([]File, error) {
	if len(input) < 1 || len(input) > maxValidationFiles {
		return nil, ErrInvalidValidation
	}
	result := make([]File, len(input))
	seen := make(map[string]struct{}, len(input))
	totalBytes := 0
	for index, file := range input {
		normalized, err := normalizePath(file.Path)
		if err != nil || len(file.Content) > maxFileContentBytes || !utf8.ValidString(file.Content) {
			return nil, ErrInvalidValidation
		}
		if _, exists := seen[normalized]; exists {
			return nil, ErrInvalidValidation
		}
		seen[normalized] = struct{}{}
		totalBytes += len(file.Content)
		if totalBytes > maxTotalContentBytes {
			return nil, ErrInvalidValidation
		}
		result[index] = File{Path: normalized, Content: file.Content}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func normalizePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") {
		return "", ErrInvalidValidation
	}
	firstPart := strings.SplitN(value, "/", 2)[0]
	if strings.Contains(firstPart, ":") {
		return "", ErrInvalidValidation
	}
	normalized := path.Clean(value)
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", ErrInvalidValidation
	}
	return normalized, nil
}

func hashFiles(files []File) (string, error) {
	encoded, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func compilePattern(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return compiled
}

func validCategory(value string) bool {
	return value == CategoryDirectory || value == CategoryNaming || value == CategoryComment || value == CategoryTesting || value == CategoryWorkflow
}

func validLevel(value string) bool {
	return value == LevelGuidance || value == LevelTemplate || value == LevelCheck || value == LevelBlock
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("standard-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
