package standards

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/0001_create_standards.sql
var standardsSchema string

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open standards PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect standards PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, standardsSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize standards schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) CreatePackage(ctx context.Context, standard Package) (Package, error) {
	rulesJSON, err := json.Marshal(standard.Rules)
	if err != nil {
		return Package{}, fmt.Errorf("encode standard package rules: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Package{}, fmt.Errorf("begin standard package creation: %w", err)
	}
	defer tx.Rollback()
	identity := standard.Scope + ":" + standard.Selector + ":" + standard.Name
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", identity); err != nil {
		return Package{}, fmt.Errorf("lock standard package identity: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM standard_packages
		WHERE scope = $1 AND selector = $2 AND name = $3`,
		standard.Scope, standard.Selector, standard.Name,
	).Scan(&standard.Version); err != nil {
		return Package{}, fmt.Errorf("allocate standard package version: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO standard_packages (
			id, name, description, scope, selector, owner, version,
			definition_hash, rules, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		standard.ID, standard.Name, standard.Description, standard.Scope,
		standard.Selector, standard.Owner, standard.Version, standard.DefinitionHash,
		rulesJSON, standard.CreatedBy, standard.CreatedAt,
	)
	if err != nil {
		return Package{}, fmt.Errorf("create standard package: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Package{}, fmt.Errorf("commit standard package: %w", err)
	}
	return standard, nil
}

func (s *PostgresStore) GetPackage(ctx context.Context, id string) (Package, error) {
	return scanPackage(s.db.QueryRowContext(ctx, `
		SELECT id, name, description, scope, selector, owner, version,
		       definition_hash, rules, created_by, created_at
		FROM standard_packages WHERE id = $1`, id))
}

func (s *PostgresStore) ListPackages(ctx context.Context, name, scope, selector string, limit int) ([]Package, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, scope, selector, owner, version,
		       definition_hash, rules, created_by, created_at
		FROM standard_packages
		WHERE ($1 = '' OR name = $1)
		  AND ($2 = '' OR scope = $2)
		  AND ($3 = '' OR selector = $3)
		ORDER BY name, scope, selector, version DESC
		LIMIT $4`, name, scope, selector, limit)
	if err != nil {
		return nil, fmt.Errorf("list standard packages: %w", err)
	}
	defer rows.Close()
	return scanPackages(rows)
}

func (s *PostgresStore) ListApplicable(ctx context.Context, project, technology string) ([]Package, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH applicable AS (
			SELECT id, name, description, scope, selector, owner, version,
			       definition_hash, rules, created_by, created_at,
			       ROW_NUMBER() OVER (
			           PARTITION BY scope, selector, name ORDER BY version DESC
			       ) AS version_rank
			FROM standard_packages
			WHERE scope = 'common'
			   OR (scope = 'technology' AND selector = $1)
			   OR (scope = 'project' AND selector = $2)
		)
		SELECT id, name, description, scope, selector, owner, version,
		       definition_hash, rules, created_by, created_at
		FROM applicable
		WHERE version_rank = 1
		ORDER BY CASE scope WHEN 'common' THEN 1 WHEN 'technology' THEN 2 ELSE 3 END, name`,
		technology, project)
	if err != nil {
		return nil, fmt.Errorf("list applicable standard packages: %w", err)
	}
	defer rows.Close()
	return scanPackages(rows)
}

func (s *PostgresStore) SaveReport(ctx context.Context, report ValidationReport) error {
	packagesJSON, err := json.Marshal(report.Packages)
	if err != nil {
		return fmt.Errorf("encode standard report packages: %w", err)
	}
	violationsJSON, err := json.Marshal(report.Violations)
	if err != nil {
		return fmt.Errorf("encode standard report violations: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO standard_validation_reports (
			id, project, technology, input_hash, passed, rule_count,
			violation_count, blocking_count, packages, violations,
			validated_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		report.ID, report.Project, report.Technology, report.InputHash, report.Passed,
		report.RuleCount, report.ViolationCount, report.BlockingCount, packagesJSON,
		violationsJSON, report.ValidatedBy, report.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save standard validation report: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetReport(ctx context.Context, id string) (ValidationReport, error) {
	return scanReport(s.db.QueryRowContext(ctx, `
		SELECT id, project, technology, input_hash, passed, rule_count,
		       violation_count, blocking_count, packages, violations,
		       validated_by, created_at
		FROM standard_validation_reports WHERE id = $1`, id))
}

func (s *PostgresStore) ListReports(ctx context.Context, project string, limit int) ([]ValidationReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project, technology, input_hash, passed, rule_count,
		       violation_count, blocking_count, packages, violations,
		       validated_by, created_at
		FROM standard_validation_reports
		WHERE $1 = '' OR project = $1
		ORDER BY created_at DESC
		LIMIT $2`, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list standard validation reports: %w", err)
	}
	defer rows.Close()
	result := make([]ValidationReport, 0)
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate standard validation reports: %w", err)
	}
	return result, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanPackage(row rowScanner) (Package, error) {
	var standard Package
	var rulesJSON []byte
	if err := row.Scan(
		&standard.ID, &standard.Name, &standard.Description, &standard.Scope,
		&standard.Selector, &standard.Owner, &standard.Version,
		&standard.DefinitionHash, &rulesJSON, &standard.CreatedBy, &standard.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Package{}, ErrPackageNotFound
		}
		return Package{}, fmt.Errorf("scan standard package: %w", err)
	}
	if err := json.Unmarshal(rulesJSON, &standard.Rules); err != nil {
		return Package{}, fmt.Errorf("decode standard package rules: %w", err)
	}
	return standard, nil
}

func scanPackages(rows *sql.Rows) ([]Package, error) {
	result := make([]Package, 0)
	for rows.Next() {
		standard, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, standard)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate standard packages: %w", err)
	}
	return result, nil
}

func scanReport(row rowScanner) (ValidationReport, error) {
	var report ValidationReport
	var packagesJSON []byte
	var violationsJSON []byte
	if err := row.Scan(
		&report.ID, &report.Project, &report.Technology, &report.InputHash,
		&report.Passed, &report.RuleCount, &report.ViolationCount,
		&report.BlockingCount, &packagesJSON, &violationsJSON,
		&report.ValidatedBy, &report.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ValidationReport{}, ErrReportNotFound
		}
		return ValidationReport{}, fmt.Errorf("scan standard validation report: %w", err)
	}
	if err := json.Unmarshal(packagesJSON, &report.Packages); err != nil {
		return ValidationReport{}, fmt.Errorf("decode standard report packages: %w", err)
	}
	if err := json.Unmarshal(violationsJSON, &report.Violations); err != nil {
		return ValidationReport{}, fmt.Errorf("decode standard report violations: %w", err)
	}
	return report, nil
}
