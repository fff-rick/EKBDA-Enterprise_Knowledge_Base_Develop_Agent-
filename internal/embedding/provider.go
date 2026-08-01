package embedding

import "context"

type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Name() string
}
