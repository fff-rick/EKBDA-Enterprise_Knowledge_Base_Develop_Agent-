package access

import (
	"context"
	"errors"
)

var (
	ErrPolicyNotFound = errors.New("project access policy not found")
	ErrInvalidPolicy  = errors.New("invalid project access policy")
	ErrAccessDenied   = errors.New("project access denied")
)

type Store interface {
	CreatePolicy(context.Context, Policy) (Policy, error)
	GetLatest(context.Context, string) (Policy, error)
	ListPolicies(context.Context, string, int) ([]Policy, error)
}
