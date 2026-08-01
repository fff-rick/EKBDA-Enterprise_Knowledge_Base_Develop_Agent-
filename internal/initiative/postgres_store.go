package initiative

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

//go:embed migrations/0001_create_project_packages.sql
var projectPackageSchema string

//go:embed migrations/0002_create_project_package_reviews.sql
var projectPackageReviewSchema string

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open project package PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect project package PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, projectPackageSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize project package schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, projectPackageReviewSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize project package review schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Create(ctx context.Context, projectPackage Package) (Package, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Package{}, fmt.Errorf("begin project package creation: %w", err)
	}
	defer tx.Rollback()
	identity := projectPackage.Project + ":" + projectPackage.Name
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", identity); err != nil {
		return Package{}, fmt.Errorf("lock project package identity: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1 FROM project_packages
		WHERE project=$1 AND name=$2`, projectPackage.Project, projectPackage.Name).Scan(&projectPackage.Version); err != nil {
		return Package{}, fmt.Errorf("allocate project package version: %w", err)
	}
	payload, err := json.Marshal(projectPackage)
	if err != nil {
		return Package{}, fmt.Errorf("encode project package: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO project_packages (
			id, project, repository, name, version, planning_session_id,
			definition_hash, payload, created_by, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		projectPackage.ID, projectPackage.Project, projectPackage.Repository, projectPackage.Name,
		projectPackage.Version, projectPackage.Source.PlanningSessionID, projectPackage.DefinitionHash,
		payload, projectPackage.CreatedBy, projectPackage.CreatedAt)
	if err != nil {
		return Package{}, fmt.Errorf("create project package: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Package{}, fmt.Errorf("commit project package creation: %w", err)
	}
	return projectPackage, nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Package, error) {
	return scanPackage(s.db.QueryRowContext(ctx, `SELECT payload FROM project_packages WHERE id=$1`, id))
}

func (s *PostgresStore) List(ctx context.Context, project, name string, limit int) ([]Package, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT payload FROM project_packages
		WHERE project=$1 AND ($2='' OR name=$2)
		ORDER BY name, version DESC LIMIT $3`, project, name, limit)
	if err != nil {
		return nil, fmt.Errorf("list project packages: %w", err)
	}
	defer rows.Close()
	result := make([]Package, 0)
	for rows.Next() {
		projectPackage, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, projectPackage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project packages: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) CreateReview(ctx context.Context, review ArtifactReview) (ArtifactReview, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ArtifactReview{}, fmt.Errorf("begin project package review: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", review.PackageID+":"+review.ArtifactType); err != nil {
		return ArtifactReview{}, fmt.Errorf("lock project package review: %w", err)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM project_packages WHERE id=$1)`, review.PackageID).Scan(&exists); err != nil {
		return ArtifactReview{}, fmt.Errorf("check project package for review: %w", err)
	}
	if !exists {
		return ArtifactReview{}, ErrPackageNotFound
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1
		FROM project_package_artifact_reviews
		WHERE package_id=$1 AND artifact_type=$2`, review.PackageID, review.ArtifactType).Scan(&review.Sequence); err != nil {
		return ArtifactReview{}, fmt.Errorf("allocate project package review sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_package_artifact_reviews (
			id, package_id, artifact_type, package_hash, sequence, decision, comment, reviewed_by, reviewed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		review.ID, review.PackageID, review.ArtifactType, review.PackageHash, review.Sequence,
		review.Decision, review.Comment, review.ReviewedBy, review.ReviewedAt); err != nil {
		return ArtifactReview{}, fmt.Errorf("create project package review: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ArtifactReview{}, fmt.Errorf("commit project package review: %w", err)
	}
	return review, nil
}

func (s *PostgresStore) ListReviews(ctx context.Context, packageID, artifactType string, limit int) ([]ArtifactReview, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM project_packages WHERE id=$1)`, packageID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check project package for reviews: %w", err)
	}
	if !exists {
		return nil, ErrPackageNotFound
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, package_id, artifact_type, package_hash, sequence, decision, comment, reviewed_by, reviewed_at
		FROM project_package_artifact_reviews
		WHERE package_id=$1 AND ($2='' OR artifact_type=$2)
		ORDER BY artifact_type, sequence DESC LIMIT $3`, packageID, artifactType, limit)
	if err != nil {
		return nil, fmt.Errorf("list project package reviews: %w", err)
	}
	defer rows.Close()
	result := make([]ArtifactReview, 0)
	for rows.Next() {
		var review ArtifactReview
		if err := rows.Scan(&review.ID, &review.PackageID, &review.ArtifactType, &review.PackageHash, &review.Sequence, &review.Decision, &review.Comment, &review.ReviewedBy, &review.ReviewedAt); err != nil {
			return nil, fmt.Errorf("scan project package review: %w", err)
		}
		result = append(result, review)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project package reviews: %w", err)
	}
	return result, nil
}

type rowScanner interface{ Scan(...any) error }

func scanPackage(row rowScanner) (Package, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Package{}, ErrPackageNotFound
		}
		return Package{}, fmt.Errorf("scan project package: %w", err)
	}
	var projectPackage Package
	if err := json.Unmarshal(payload, &projectPackage); err != nil {
		return Package{}, fmt.Errorf("decode project package: %w", err)
	}
	return projectPackage, nil
}
