package repositorysync

import (
	"context"
	"errors"
)

var (
	ErrDirtyRepository = errors.New("repository knowledge sync requires a clean Git worktree")
	ErrInvalidInput    = errors.New("invalid repository knowledge sync request")
	ErrReportNotFound  = errors.New("repository knowledge sync report not found")
	ErrSyncInProgress  = errors.New("repository knowledge sync is already in progress")
)

type Store interface {
	Save(context.Context, Report) error
	Get(context.Context, string) (Report, error)
	List(context.Context, string, int) ([]Report, error)
	LatestCompleted(context.Context, string, string) (Report, error)
}
