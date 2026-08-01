package ingestion

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/0001_create_ingestion_jobs.sql
var jobSchema string

type PostgresJobStore struct {
	db *sql.DB
}

func NewPostgresJobStore(ctx context.Context, dsn string) (*PostgresJobStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open ingestion PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect ingestion PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, jobSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize ingestion schema: %w", err)
	}
	return &PostgresJobStore{db: db}, nil
}

func (s *PostgresJobStore) Create(ctx context.Context, report Report) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ingestion_jobs (
			id, status, root, project, scanned, created, updated, skipped,
			deleted, failed, error, started_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		report.ID,
		report.Status,
		report.Root,
		report.Project,
		report.Scanned,
		report.Created,
		report.Updated,
		report.Skipped,
		report.Deleted,
		report.Failed,
		report.Error,
		report.StartedAt,
		nullTime(report.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("create ingestion job: %w", err)
	}
	return nil
}

func (s *PostgresJobStore) Update(ctx context.Context, report Report, file *FileResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ingestion job update: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE ingestion_jobs
		SET status = $2, scanned = $3, created = $4, updated = $5,
		    skipped = $6, deleted = $7, failed = $8, error = $9,
		    completed_at = $10
		WHERE id = $1`,
		report.ID,
		report.Status,
		report.Scanned,
		report.Created,
		report.Updated,
		report.Skipped,
		report.Deleted,
		report.Failed,
		report.Error,
		nullTime(report.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("update ingestion job: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read ingestion job update count: %w", err)
	}
	if rowsAffected == 0 {
		return ErrJobNotFound
	}
	if file != nil {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO ingestion_job_files (
				job_id, path, action, document_id, version, error
			) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (job_id, path) DO UPDATE SET
				action = EXCLUDED.action,
				document_id = EXCLUDED.document_id,
				version = EXCLUDED.version,
				error = EXCLUDED.error`,
			report.ID,
			file.Path,
			file.Action,
			file.DocumentID,
			file.Version,
			file.Error,
		)
		if err != nil {
			return fmt.Errorf("save ingestion job file: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ingestion job update: %w", err)
	}
	return nil
}

func (s *PostgresJobStore) Get(ctx context.Context, id string) (Report, error) {
	var report Report
	var completedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, status, root, project, scanned, created, updated, skipped,
		       deleted, failed, error, started_at, completed_at
		FROM ingestion_jobs
		WHERE id = $1`, id).Scan(
		&report.ID,
		&report.Status,
		&report.Root,
		&report.Project,
		&report.Scanned,
		&report.Created,
		&report.Updated,
		&report.Skipped,
		&report.Deleted,
		&report.Failed,
		&report.Error,
		&report.StartedAt,
		&completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Report{}, ErrJobNotFound
		}
		return Report{}, fmt.Errorf("get ingestion job: %w", err)
	}
	if completedAt.Valid {
		report.CompletedAt = completedAt.Time
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT path, action, document_id, version, error
		FROM ingestion_job_files
		WHERE job_id = $1
		ORDER BY path`, id)
	if err != nil {
		return Report{}, fmt.Errorf("list ingestion job files: %w", err)
	}
	defer rows.Close()
	report.Files = make([]FileResult, 0)
	for rows.Next() {
		var file FileResult
		if err := rows.Scan(&file.Path, &file.Action, &file.DocumentID, &file.Version, &file.Error); err != nil {
			return Report{}, fmt.Errorf("scan ingestion job file: %w", err)
		}
		report.Files = append(report.Files, file)
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("iterate ingestion job files: %w", err)
	}
	return report, nil
}

func (s *PostgresJobStore) Close() error {
	return s.db.Close()
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
