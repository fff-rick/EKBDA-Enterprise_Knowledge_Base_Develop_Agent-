package answer

import "context"

type Provider interface {
	Generate(ctx context.Context, query string, evidence []Evidence) (Draft, error)
	Name() string
}
