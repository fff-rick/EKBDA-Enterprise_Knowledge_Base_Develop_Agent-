package repositorysync

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

//go:embed migrations/0001_create_repository_knowledge_syncs.sql
var repositorySyncSchema string

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open repository sync PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect repository sync PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, repositorySyncSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize repository sync schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Save(ctx context.Context, report Report) error {
	changes, err := json.Marshal(report.CommitChanges)
	if err != nil {
		return fmt.Errorf("encode repository sync commit changes: %w", err)
	}
	files, err := json.Marshal(report.Files)
	if err != nil {
		return fmt.Errorf("encode repository sync files: %w", err)
	}
	allowedRoles, err := json.Marshal(report.AllowedRoles)
	if err != nil {
		return fmt.Errorf("encode repository sync allowed roles: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO repository_knowledge_sync_reports (
			id, status, repository, project, business_domain, classification, allowed_roles,
			head_commit, previous_head_commit,
			branch, full_resync, commit_changes, scanned, created, updated,
			skipped, deleted, failed, sensitive_files_skipped, redaction_count,
			files, synced_by, started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		report.ID, report.Status, report.Repository, report.Project, report.BusinessDomain,
		report.Classification, allowedRoles, report.HeadCommit, report.PreviousHeadCommit,
		report.Branch, report.FullResync, changes, report.Scanned, report.Created,
		report.Updated, report.Skipped, report.Deleted, report.Failed,
		report.SensitiveFilesSkipped, report.RedactionCount, files, report.SyncedBy,
		report.StartedAt, report.CompletedAt)
	if err != nil {
		return fmt.Errorf("save repository knowledge sync report: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Report, error) {
	return scanReport(s.db.QueryRowContext(ctx, selectReport+` WHERE id = $1`, id))
}

func (s *PostgresStore) List(ctx context.Context, project string, limit int) ([]Report, error) {
	rows, err := s.db.QueryContext(ctx, selectReport+`
		WHERE $1 = '' OR project = $1 ORDER BY started_at DESC LIMIT $2`, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list repository knowledge sync reports: %w", err)
	}
	defer rows.Close()
	result := make([]Report, 0)
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repository knowledge sync reports: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) LatestCompleted(ctx context.Context, project, repository string) (Report, error) {
	return scanReport(s.db.QueryRowContext(ctx, selectReport+`
		WHERE project = $1 AND repository = $2 AND status = 'completed'
		ORDER BY completed_at DESC LIMIT 1`, project, repository))
}

const selectReport = `
	SELECT id, status, repository, project, business_domain, classification, allowed_roles,
	       head_commit, previous_head_commit,
	       branch, full_resync, commit_changes, scanned, created, updated,
	       skipped, deleted, failed, sensitive_files_skipped, redaction_count,
	       files, synced_by, started_at, completed_at
	FROM repository_knowledge_sync_reports`

type rowScanner interface{ Scan(...any) error }

func scanReport(row rowScanner) (Report, error) {
	var report Report
	var allowedRoles, changes, files []byte
	if err := row.Scan(
		&report.ID, &report.Status, &report.Repository, &report.Project,
		&report.BusinessDomain, &report.Classification, &allowedRoles,
		&report.HeadCommit, &report.PreviousHeadCommit, &report.Branch,
		&report.FullResync, &changes, &report.Scanned, &report.Created,
		&report.Updated, &report.Skipped, &report.Deleted, &report.Failed,
		&report.SensitiveFilesSkipped, &report.RedactionCount, &files,
		&report.SyncedBy, &report.StartedAt, &report.CompletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Report{}, ErrReportNotFound
		}
		return Report{}, fmt.Errorf("scan repository knowledge sync report: %w", err)
	}
	if err := json.Unmarshal(allowedRoles, &report.AllowedRoles); err != nil {
		return Report{}, fmt.Errorf("decode repository sync allowed roles: %w", err)
	}
	if err := json.Unmarshal(changes, &report.CommitChanges); err != nil {
		return Report{}, fmt.Errorf("decode repository sync commit changes: %w", err)
	}
	if err := json.Unmarshal(files, &report.Files); err != nil {
		return Report{}, fmt.Errorf("decode repository sync files: %w", err)
	}
	return report, nil
}
