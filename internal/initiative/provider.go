package initiative

import (
	"context"

	"ekbda/internal/planning"
)

type Provider interface {
	Build(context.Context, planning.Session) (BuildOutput, error)
	Name() string
}

type BuildOutput struct {
	Artifacts    []Artifact    `json:"artifacts"`
	Traceability []TraceRecord `json:"traceability"`
}
