package planning

import (
	"context"
	"errors"
)

var (
	ErrInvalidInput          = errors.New("invalid planning session input")
	ErrSessionNotFound       = errors.New("planning session not found")
	ErrRevisionConflict      = errors.New("planning session revision conflict")
	ErrInvalidTransition     = errors.New("invalid planning session transition")
	ErrIncompleteAnswers     = errors.New("all clarification questions require answers")
	ErrIncompleteResolutions = errors.New("all role review decisions require resolutions")
	ErrForbiddenParticipant  = errors.New("planning session action is not allowed for this user")
	ErrSelfApproval          = errors.New("planning session creator cannot approve their own plan")
	ErrSelfResolution        = errors.New("planning session creator cannot resolve role review decisions")
	ErrProviderFailed        = errors.New("planning provider request failed")
	ErrInvalidProviderOutput = errors.New("planning provider returned an invalid result")
)

type Store interface {
	Create(context.Context, Session, Event) error
	Update(context.Context, Session, int, Event) error
	Get(context.Context, string) (Session, error)
	List(context.Context, string, int) ([]Session, error)
	ListEvents(context.Context, string) ([]Event, error)
}
