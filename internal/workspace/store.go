package workspace

import (
	"context"
	"errors"
)

var ErrSnapshotNotFound = errors.New("workspace validation snapshot not found")

type Store interface {
	Save(context.Context, Snapshot) error
	Get(context.Context, string) (Snapshot, error)
	List(context.Context, string, int) ([]Snapshot, error)
}
