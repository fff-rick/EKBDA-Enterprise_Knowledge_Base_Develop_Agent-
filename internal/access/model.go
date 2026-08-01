package access

import "time"

const (
	ModeDisabled = "disabled"
	ModeEnforced = "enforced"
)

type CreatePolicyInput struct {
	Project      string   `json:"project"`
	Description  string   `json:"description"`
	Owner        string   `json:"owner"`
	Users        []string `json:"users"`
	Roles        []string `json:"roles"`
	Repositories []string `json:"repositories"`
}

// Policy is an immutable project authorization revision. The latest revision
// for a project is the active policy.
type Policy struct {
	ID             string    `json:"id"`
	Project        string    `json:"project"`
	Description    string    `json:"description"`
	Owner          string    `json:"owner"`
	Version        int       `json:"version"`
	DefinitionHash string    `json:"definition_hash"`
	Users          []string  `json:"users"`
	Roles          []string  `json:"roles"`
	Repositories   []string  `json:"repositories"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}
