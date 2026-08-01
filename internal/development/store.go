package development

import (
	"context"
	"errors"
)

var (
	ErrInvalidInput       = errors.New("invalid development session input")
	ErrSessionNotFound    = errors.New("development session not found")
	ErrRevisionConflict   = errors.New("development session revision conflict")
	ErrInvalidTransition  = errors.New("invalid development session transition")
	ErrDirtyWorkspace     = errors.New("development session requires a clean Git workspace")
	ErrMissingBaseline    = errors.New("development session requires an existing Git HEAD commit")
	ErrBaselineChanged    = errors.New("development session Git baseline changed")
	ErrPackageNotApproved = errors.New("all project package artifacts require current approval")
	ErrInvalidPatch       = errors.New("patch failed safety validation")
	ErrPathNotAllowed     = errors.New("patch changes a path outside the approved scope")
	ErrSensitiveContent   = errors.New("patch contains a sensitive path or suspected secret")
	ErrCommandNotAllowed  = errors.New("proposal requests a command outside the approved allowlist")
	ErrForbiddenActor     = errors.New("development session action is not allowed for this user")
	ErrSelfApproval       = errors.New("development session creator cannot approve their own proposal")
	ErrExecutionDisabled  = errors.New("controlled development execution is disabled")
	ErrExecutionConflict  = errors.New("development execution confirmation does not match the approved proposal")
	ErrDeliveryDisabled   = errors.New("controlled development delivery is disabled")
	ErrDeliveryConflict   = errors.New("development delivery confirmation does not match the verified proposal")
	ErrSelfDelivery       = errors.New("development session creator cannot deliver their own proposal")
)

type Store interface {
	Create(context.Context, Session, Event) error
	Update(context.Context, Session, int, Event) error
	Get(context.Context, string) (Session, error)
	List(context.Context, string, int) ([]Session, error)
	ListEvents(context.Context, string) ([]Event, error)
	ListExecuting(context.Context, int) ([]Session, error)
	ListDelivering(context.Context, int) ([]Session, error)
}
