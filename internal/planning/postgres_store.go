package planning

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

//go:embed migrations/0001_create_planning_sessions.sql
var planningSchema string

//go:embed migrations/0002_add_role_review_statuses.sql
var roleReviewSchema string

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open planning PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect planning PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, planningSchema+"\n"+roleReviewSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize planning schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Create(ctx context.Context, session Session, event Event) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode planning session: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin planning session creation: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO planning_sessions (id, project, repository, status, revision, payload, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		session.ID, session.Project, session.Repository, session.Status, session.Revision, payload, session.CreatedAt, session.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create planning session: %w", err)
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit planning session creation: %w", err)
	}
	return nil
}

func (s *PostgresStore) Update(ctx context.Context, session Session, expectedRevision int, event Event) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode planning session: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin planning session update: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE planning_sessions
		SET status=$1, revision=$2, payload=$3, updated_at=$4
		WHERE id=$5 AND revision=$6`,
		session.Status, session.Revision, payload, session.UpdatedAt, session.ID, expectedRevision)
	if err != nil {
		return fmt.Errorf("update planning session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect planning session update: %w", err)
	}
	if rows != 1 {
		return ErrRevisionConflict
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit planning session update: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Session, error) {
	return scanSession(s.db.QueryRowContext(ctx, `SELECT payload FROM planning_sessions WHERE id=$1`, id))
}

func (s *PostgresStore) List(ctx context.Context, project string, limit int) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT payload FROM planning_sessions
		WHERE $1='' OR project=$1 ORDER BY created_at DESC LIMIT $2`, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list planning sessions: %w", err)
	}
	defer rows.Close()
	result := make([]Session, 0)
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning sessions: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) ListEvents(ctx context.Context, sessionID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, sequence, event_type, from_status, to_status, actor, reason, created_at
		FROM planning_session_events WHERE session_id=$1 ORDER BY sequence`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list planning session events: %w", err)
	}
	defer rows.Close()
	result := make([]Event, 0)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.SessionID, &event.Sequence, &event.Type, &event.FromStatus, &event.ToStatus, &event.Actor, &event.Reason, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan planning session event: %w", err)
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning session events: %w", err)
	}
	if len(result) == 0 {
		if _, err := s.Get(ctx, sessionID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type rowScanner interface{ Scan(...any) error }

func scanSession(row rowScanner) (Session, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("scan planning session: %w", err)
	}
	var session Session
	if err := json.Unmarshal(payload, &session); err != nil {
		return Session{}, fmt.Errorf("decode planning session: %w", err)
	}
	return session, nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, event Event) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO planning_session_events (
			id, session_id, sequence, event_type, from_status, to_status, actor, reason, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		event.ID, event.SessionID, event.Sequence, event.Type, event.FromStatus,
		event.ToStatus, event.Actor, event.Reason, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("create planning session event: %w", err)
	}
	return nil
}
