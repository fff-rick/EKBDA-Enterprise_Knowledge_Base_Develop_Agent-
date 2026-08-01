package ingestion

import (
	"context"
	"errors"
)

var ErrJobNotFound = errors.New("ingestion job not found")

type JobStore interface {
	Create(ctx context.Context, report Report) error
	Update(ctx context.Context, report Report, file *FileResult) error
	Get(ctx context.Context, id string) (Report, error)
}
