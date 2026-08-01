package initiative

import (
	"context"
	"errors"
)

var (
	ErrInvalidInput          = errors.New("invalid project package input")
	ErrPackageNotFound       = errors.New("project package not found")
	ErrPlanningNotApproved   = errors.New("planning session must be approved before creating a project package")
	ErrPackageHashConflict   = errors.New("project package definition hash does not match")
	ErrInvalidReview         = errors.New("invalid project package artifact review")
	ErrInvalidProviderOutput = errors.New("project package provider returned an invalid result")
	ErrProviderFailed        = errors.New("project package provider request failed")
)

type Store interface {
	Create(context.Context, Package) (Package, error)
	Get(context.Context, string) (Package, error)
	List(context.Context, string, string, int) ([]Package, error)
	CreateReview(context.Context, ArtifactReview) (ArtifactReview, error)
	ListReviews(context.Context, string, string, int) ([]ArtifactReview, error)
}
