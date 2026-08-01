package workspace

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

//go:embed migrations/0001_create_workspace_snapshots.sql
var workspaceSchema string

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open workspace PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect workspace PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, workspaceSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize workspace schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Save(ctx context.Context, snapshot Snapshot) error {
	changesJSON, err := json.Marshal(snapshot.Changes)
	if err != nil {
		return fmt.Errorf("encode workspace changes: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO workspace_validation_snapshots (
			id, repository, project, technology, head_commit, branch, dirty,
			file_count, tracked_count, untracked_count, binary_count,
			changed_count, changes, input_hash, standards_report_id, passed,
			validated_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
		          $14, $15, $16, $17, $18)`,
		snapshot.ID, snapshot.Repository, snapshot.Project, snapshot.Technology,
		snapshot.HeadCommit, snapshot.Branch, snapshot.Dirty, snapshot.FileCount,
		snapshot.TrackedCount, snapshot.UntrackedCount, snapshot.BinaryCount,
		snapshot.ChangedCount, changesJSON, snapshot.InputHash,
		snapshot.StandardsReportID, snapshot.Passed, snapshot.ValidatedBy, snapshot.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save workspace validation snapshot: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Snapshot, error) {
	return scanSnapshot(s.db.QueryRowContext(ctx, `
		SELECT id, repository, project, technology, head_commit, branch, dirty,
		       file_count, tracked_count, untracked_count, binary_count,
		       changed_count, changes, input_hash, standards_report_id, passed,
		       validated_by, created_at
		FROM workspace_validation_snapshots WHERE id = $1`, id))
}

func (s *PostgresStore) List(ctx context.Context, project string, limit int) ([]Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repository, project, technology, head_commit, branch, dirty,
		       file_count, tracked_count, untracked_count, binary_count,
		       changed_count, changes, input_hash, standards_report_id, passed,
		       validated_by, created_at
		FROM workspace_validation_snapshots
		WHERE $1 = '' OR project = $1
		ORDER BY created_at DESC
		LIMIT $2`, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list workspace validation snapshots: %w", err)
	}
	defer rows.Close()
	result := make([]Snapshot, 0)
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace validation snapshots: %w", err)
	}
	return result, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanSnapshot(row rowScanner) (Snapshot, error) {
	var snapshot Snapshot
	var changesJSON []byte
	if err := row.Scan(
		&snapshot.ID, &snapshot.Repository, &snapshot.Project, &snapshot.Technology,
		&snapshot.HeadCommit, &snapshot.Branch, &snapshot.Dirty, &snapshot.FileCount,
		&snapshot.TrackedCount, &snapshot.UntrackedCount, &snapshot.BinaryCount,
		&snapshot.ChangedCount, &changesJSON, &snapshot.InputHash,
		&snapshot.StandardsReportID, &snapshot.Passed, &snapshot.ValidatedBy,
		&snapshot.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrSnapshotNotFound
		}
		return Snapshot{}, fmt.Errorf("scan workspace validation snapshot: %w", err)
	}
	if err := json.Unmarshal(changesJSON, &snapshot.Changes); err != nil {
		return Snapshot{}, fmt.Errorf("decode workspace changes: %w", err)
	}
	return snapshot, nil
}
