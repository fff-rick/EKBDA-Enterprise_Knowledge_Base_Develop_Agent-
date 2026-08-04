package release

import (
	"context"
	"errors"
)

var (
	ErrInvalidInput         = errors.New("invalid release request input")
	ErrNotFound             = errors.New("release request not found")
	ErrRevisionConflict     = errors.New("release request revision conflict")
	ErrInvalidTransition    = errors.New("invalid release request transition")
	ErrDevelopmentNotReady  = errors.New("development session is not delivered")
	ErrSelfApproval         = errors.New("release request creator cannot approve their own release")
	ErrSelfRollbackApproval = errors.New("rollback requester cannot approve their own rollback")
	ErrConfirmation         = errors.New("release confirmation does not match approved immutable inputs")
	ErrDisabled             = errors.New("controlled release integration is disabled")
	ErrProviderConflict     = errors.New("provider event identity conflict")
	ErrProviderEvidence     = errors.New("provider success event is missing required trusted evidence")
	ErrProviderRejected     = errors.New("CI/CD provider rejected the release request")
)

type Store interface {
	Create(context.Context, Request, Event) error
	Update(context.Context, Request, int, Event) error
	ApplyProviderEvent(context.Context, Request, int, Event, string, string) (bool, error)
	Get(context.Context, string) (Request, error)
	List(context.Context, string, int) ([]Request, error)
	ListEvents(context.Context, string) ([]Event, error)
}

type DevelopmentReader interface {
	Get(context.Context, string) (DevelopmentSession, error)
}

type DevelopmentReaderFunc func(context.Context, string) (DevelopmentSession, error)

func (f DevelopmentReaderFunc) Get(ctx context.Context, id string) (DevelopmentSession, error) {
	return f(ctx, id)
}

type DevelopmentSession struct {
	ID             string
	Project        string
	Repository     string
	Status         string
	DeliveryStatus string
	Commit         string
	PullRequestURL string
}

type Connector interface {
	Enabled() bool
	Trigger(context.Context, TriggerRequest) (ProviderRun, error)
	Rollback(context.Context, RollbackRequest) (ProviderRun, error)
}
