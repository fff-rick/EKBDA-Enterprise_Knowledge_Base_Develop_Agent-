package standards

import "time"

const (
	ScopeCommon     = "common"
	ScopeTechnology = "technology"
	ScopeProject    = "project"

	CategoryDirectory = "directory"
	CategoryNaming    = "naming"
	CategoryComment   = "comment"
	CategoryTesting   = "testing"
	CategoryWorkflow  = "workflow"

	LevelGuidance = "guidance"
	LevelTemplate = "template"
	LevelCheck    = "check"
	LevelBlock    = "block"

	CheckRequiredPath  = "required_path"
	CheckForbiddenPath = "forbidden_path"
	CheckPathPattern   = "path_pattern"
	CheckContent       = "content_required"
	CheckMinimumMatch  = "minimum_matches"
)

type CreatePackageInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	Selector    string `json:"selector"`
	Owner       string `json:"owner"`
	Rules       []Rule `json:"rules"`
}

type Rule struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Owner       string     `json:"owner"`
	Category    string     `json:"category"`
	Level       string     `json:"level"`
	Check       *RuleCheck `json:"check,omitempty"`
}

type RuleCheck struct {
	Type    string `json:"type"`
	Target  string `json:"target,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Minimum int    `json:"minimum,omitempty"`
}

type Package struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Scope          string    `json:"scope"`
	Selector       string    `json:"selector"`
	Owner          string    `json:"owner"`
	Version        int       `json:"version"`
	DefinitionHash string    `json:"definition_hash"`
	Rules          []Rule    `json:"rules"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

type PackageSummary struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Scope          string    `json:"scope"`
	Selector       string    `json:"selector"`
	Owner          string    `json:"owner"`
	Version        int       `json:"version"`
	DefinitionHash string    `json:"definition_hash"`
	RuleCount      int       `json:"rule_count"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func (p Package) Summary() PackageSummary {
	return PackageSummary{
		ID: p.ID, Name: p.Name, Description: p.Description, Scope: p.Scope,
		Selector: p.Selector, Owner: p.Owner, Version: p.Version,
		DefinitionHash: p.DefinitionHash, RuleCount: len(p.Rules),
		CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt,
	}
}

type File struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
}

type ValidateInput struct {
	Project    string `json:"project"`
	Technology string `json:"technology"`
	Files      []File `json:"files"`
}

type PackageReference struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Scope          string `json:"scope"`
	Selector       string `json:"selector"`
	Version        int    `json:"version"`
	DefinitionHash string `json:"definition_hash"`
}

type Violation struct {
	RuleID         string `json:"rule_id"`
	RuleTitle      string `json:"rule_title"`
	Category       string `json:"category"`
	Level          string `json:"level"`
	PackageID      string `json:"package_id"`
	PackageName    string `json:"package_name"`
	PackageVersion int    `json:"package_version"`
	Path           string `json:"path,omitempty"`
	Message        string `json:"message"`
}

type ValidationReport struct {
	ID             string             `json:"id"`
	Project        string             `json:"project"`
	Technology     string             `json:"technology"`
	InputHash      string             `json:"input_hash"`
	Passed         bool               `json:"passed"`
	RuleCount      int                `json:"rule_count"`
	ViolationCount int                `json:"violation_count"`
	BlockingCount  int                `json:"blocking_count"`
	Packages       []PackageReference `json:"packages"`
	Violations     []Violation        `json:"violations"`
	ValidatedBy    string             `json:"validated_by"`
	CreatedAt      time.Time          `json:"created_at"`
}
