package evaluation

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSuiteNotFound     = errors.New("evaluation suite not found")
	ErrRunNotFound       = errors.New("evaluation run not found")
	ErrRunNotClaimed     = errors.New("evaluation run is not owned by this worker")
	ErrRunAlreadyRetried = errors.New("evaluation run already has a retry")
)

type Store interface {
	CreateSuite(ctx context.Context, suite Suite) (Suite, error)
	GetSuite(ctx context.Context, id string) (Suite, error)
	ListSuites(ctx context.Context, name string, limit int) ([]Suite, error)
	CreateRun(ctx context.Context, run Run) error
	UpdateRun(ctx context.Context, run Run) error
	GetRun(ctx context.Context, id string) (Run, error)
	ListRuns(ctx context.Context, suiteID string, limit int) ([]Run, error)
	ListRunnable(ctx context.Context, now time.Time, limit int) ([]string, error)
	ClaimRun(ctx context.Context, id, workerID string, now, leaseUntil time.Time) (Run, bool, error)
	RenewLease(ctx context.Context, id, workerID string, leaseUntil time.Time) error
	RequestCancel(ctx context.Context, id string, now time.Time) (Run, error)
	CompleteRun(ctx context.Context, run Run, workerID string) error
}
