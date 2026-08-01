package agenttask

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTaskNotFound       = errors.New("agent task not found")
	ErrTaskNotClaimed     = errors.New("agent task is not owned by this worker")
	ErrTaskAlreadyRetried = errors.New("agent task already has a retry")
)

type Store interface {
	Create(context.Context, Task) error
	Get(context.Context, string) (Task, error)
	List(context.Context, string, string, string, int) ([]Task, error)
	ListRunnable(context.Context, time.Time, int) ([]string, error)
	Claim(context.Context, string, string, time.Time, time.Time) (Task, bool, error)
	RenewLease(context.Context, string, string, time.Time) error
	RequestCancel(context.Context, string, time.Time) (Task, error)
	Complete(context.Context, Task, string) error
}
