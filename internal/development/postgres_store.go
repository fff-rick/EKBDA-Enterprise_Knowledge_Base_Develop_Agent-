package development

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

//go:embed migrations/0001_create_development_sessions.sql
var developmentSchema string

type PostgresStore struct{ db *sql.DB }

type storedSession struct {
	Session Session `json:"session"`
	Patch   string  `json:"patch,omitempty"`
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open development PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect development PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, developmentSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize development schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func encodeSession(session Session) ([]byte, error) {
	patch := ""
	if session.Proposal != nil {
		patch = session.Proposal.Patch
	}
	return json.Marshal(storedSession{Session: session, Patch: patch})
}

func (s *PostgresStore) Create(ctx context.Context, session Session, event Event) error {
	payload, err := encodeSession(session)
	if err != nil {
		return fmt.Errorf("encode development session: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin development session creation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO development_sessions (id,project,repository,status,revision,payload,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, session.ID, session.Project, session.Repository, session.Status, session.Revision, payload, session.CreatedAt, session.UpdatedAt); err != nil {
		return fmt.Errorf("create development session: %w", err)
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit development session creation: %w", err)
	}
	return nil
}

func (s *PostgresStore) Update(ctx context.Context, session Session, expectedRevision int, event Event) error {
	payload, err := encodeSession(session)
	if err != nil {
		return fmt.Errorf("encode development session: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin development session update: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE development_sessions SET status=$1,revision=$2,payload=$3,updated_at=$4 WHERE id=$5 AND revision=$6`, session.Status, session.Revision, payload, session.UpdatedAt, session.ID, expectedRevision)
	if err != nil {
		return fmt.Errorf("update development session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect development session update: %w", err)
	}
	if rows != 1 {
		return ErrRevisionConflict
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit development session update: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Session, error) {
	return scanSession(s.db.QueryRowContext(ctx, `SELECT payload FROM development_sessions WHERE id=$1`, id))
}

func (s *PostgresStore) List(ctx context.Context, project string, limit int) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM development_sessions WHERE project=$1 ORDER BY created_at DESC LIMIT $2`, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list development sessions: %w", err)
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
		return nil, fmt.Errorf("iterate development sessions: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) ListEvents(ctx context.Context, sessionID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,sequence,event_type,from_status,to_status,actor,reason,created_at FROM development_session_events WHERE session_id=$1 ORDER BY sequence`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list development session events: %w", err)
	}
	defer rows.Close()
	result := make([]Event, 0)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.SessionID, &event.Sequence, &event.Type, &event.FromStatus, &event.ToStatus, &event.Actor, &event.Reason, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan development session event: %w", err)
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate development session events: %w", err)
	}
	if len(result) == 0 {
		if _, err := s.Get(ctx, sessionID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *PostgresStore) ListExecuting(ctx context.Context, limit int) ([]Session, error) {
	return s.listStatus(ctx, StatusExecuting, limit)
}

func (s *PostgresStore) ListDelivering(ctx context.Context, limit int) ([]Session, error) {
	return s.listStatus(ctx, StatusDelivering, limit)
}

func (s *PostgresStore) listStatus(ctx context.Context, status string, limit int) ([]Session, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM development_sessions WHERE status=$1 ORDER BY updated_at LIMIT $2`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list in-progress development sessions: %w", err)
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
		return nil, fmt.Errorf("iterate in-progress development sessions: %w", err)
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
		return Session{}, fmt.Errorf("scan development session: %w", err)
	}
	var stored storedSession
	if err := json.Unmarshal(payload, &stored); err != nil {
		return Session{}, fmt.Errorf("decode development session: %w", err)
	}
	if stored.Session.Proposal != nil {
		stored.Session.Proposal.Patch = stored.Patch
	}
	return stored.Session, nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, event Event) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO development_session_events (id,session_id,sequence,event_type,from_status,to_status,actor,reason,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, event.ID, event.SessionID, event.Sequence, event.Type, event.FromStatus, event.ToStatus, event.Actor, event.Reason, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("create development session event: %w", err)
	}
	return nil
}
