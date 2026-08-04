package release

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

//go:embed migrations/0001_create_release_requests.sql
var releaseSchema string

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open release PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect release PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, releaseSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize release schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Create(ctx context.Context, request Request, event Event) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode release request: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO release_requests (id,project,status,revision,payload,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, request.ID, request.Project, request.Status, request.Revision, payload, request.CreatedAt, request.UpdatedAt); err != nil {
		return fmt.Errorf("create release request: %w", err)
	}
	if err := insertReleaseEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) Update(ctx context.Context, request Request, expected int, event Event) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode release request: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE release_requests SET status=$1,revision=$2,payload=$3,updated_at=$4 WHERE id=$5 AND revision=$6`, request.Status, request.Revision, payload, request.UpdatedAt, request.ID, expected)
	if err != nil {
		return fmt.Errorf("update release request: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrRevisionConflict
	}
	if err := insertReleaseEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) ApplyProviderEvent(ctx context.Context, request Request, expected int, event Event, providerEventID, payloadHash string) (bool, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("encode release request: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var currentRevision int
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM release_requests WHERE id=$1 FOR UPDATE`, request.ID).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	var existingRelease, existingHash string
	err = tx.QueryRowContext(ctx, `SELECT release_id,payload_hash FROM release_provider_receipts WHERE event_id=$1`, providerEventID).Scan(&existingRelease, &existingHash)
	if err == nil {
		if existingRelease != request.ID || existingHash != payloadHash {
			return false, ErrProviderConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if currentRevision != expected {
		return false, ErrRevisionConflict
	}
	receiptResult, err := tx.ExecContext(ctx, `INSERT INTO release_provider_receipts (event_id,release_id,payload_hash,created_at) VALUES ($1,$2,$3,$4) ON CONFLICT (event_id) DO NOTHING`, providerEventID, request.ID, payloadHash, event.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("record provider event: %w", err)
	}
	receiptRows, _ := receiptResult.RowsAffected()
	if receiptRows == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT release_id,payload_hash FROM release_provider_receipts WHERE event_id=$1`, providerEventID).Scan(&existingRelease, &existingHash); err != nil {
			return false, err
		}
		if existingRelease != request.ID || existingHash != payloadHash {
			return false, ErrProviderConflict
		}
		return false, nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE release_requests SET status=$1,revision=$2,payload=$3,updated_at=$4 WHERE id=$5 AND revision=$6`, request.Status, request.Revision, payload, request.UpdatedAt, request.ID, expected)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return false, ErrRevisionConflict
	}
	if err := insertReleaseEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Request, error) {
	return scanRelease(s.db.QueryRowContext(ctx, `SELECT payload FROM release_requests WHERE id=$1`, id))
}

func (s *PostgresStore) List(ctx context.Context, project string, limit int) ([]Request, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM release_requests WHERE project=$1 ORDER BY created_at DESC LIMIT $2`, project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Request, 0)
	for rows.Next() {
		value, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *PostgresStore) ListEvents(ctx context.Context, id string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,release_id,sequence,event_type,from_status,to_status,actor,reason,provider_event_id,created_at FROM release_events WHERE release_id=$1 ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Event, 0)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.ReleaseID, &event.Sequence, &event.Type, &event.FromStatus, &event.ToStatus, &event.Actor, &event.Reason, &event.ProviderEventID, &event.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	if len(result) == 0 {
		if _, err := s.Get(ctx, id); err != nil {
			return nil, err
		}
	}
	return result, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanRelease(row rowScanner) (Request, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Request{}, ErrNotFound
		}
		return Request{}, err
	}
	var value Request
	if err := json.Unmarshal(payload, &value); err != nil {
		return Request{}, err
	}
	return value, nil
}
func insertReleaseEvent(ctx context.Context, tx *sql.Tx, event Event) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO release_events (id,release_id,sequence,event_type,from_status,to_status,actor,reason,provider_event_id,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, event.ID, event.ReleaseID, event.Sequence, event.Type, event.FromStatus, event.ToStatus, event.Actor, event.Reason, event.ProviderEventID, event.CreatedAt)
	return err
}
