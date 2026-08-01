package standards

import (
	"context"
	"errors"
)

var (
	ErrPackageNotFound = errors.New("standard package not found")
	ErrReportNotFound  = errors.New("standard validation report not found")
)

type Store interface {
	CreatePackage(context.Context, Package) (Package, error)
	GetPackage(context.Context, string) (Package, error)
	ListPackages(context.Context, string, string, string, int) ([]Package, error)
	ListApplicable(context.Context, string, string) ([]Package, error)
	SaveReport(context.Context, ValidationReport) error
	GetReport(context.Context, string) (ValidationReport, error)
	ListReports(context.Context, string, int) ([]ValidationReport, error)
}
