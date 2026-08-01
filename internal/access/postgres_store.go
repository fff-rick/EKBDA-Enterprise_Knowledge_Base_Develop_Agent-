package access

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

//go:embed migrations/0001_create_project_access_policies.sql
var accessSchema string

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open access PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect access PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, accessSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize access schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) CreatePolicy(ctx context.Context, policy Policy) (Policy, error) {
	users, err := json.Marshal(policy.Users)
	if err != nil {
		return Policy{}, fmt.Errorf("encode access policy users: %w", err)
	}
	roles, err := json.Marshal(policy.Roles)
	if err != nil {
		return Policy{}, fmt.Errorf("encode access policy roles: %w", err)
	}
	repositories, err := json.Marshal(policy.Repositories)
	if err != nil {
		return Policy{}, fmt.Errorf("encode access policy repositories: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Policy{}, fmt.Errorf("begin access policy creation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", policy.Project); err != nil {
		return Policy{}, fmt.Errorf("lock access policy project: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM project_access_policies WHERE project = $1`, policy.Project).Scan(&policy.Version); err != nil {
		return Policy{}, fmt.Errorf("allocate access policy version: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO project_access_policies (
			id, project, description, owner, version, definition_hash,
			users, roles, repositories, created_by, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		policy.ID, policy.Project, policy.Description, policy.Owner, policy.Version,
		policy.DefinitionHash, users, roles, repositories, policy.CreatedBy, policy.CreatedAt)
	if err != nil {
		return Policy{}, fmt.Errorf("create access policy: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Policy{}, fmt.Errorf("commit access policy: %w", err)
	}
	return policy, nil
}

func (s *PostgresStore) GetLatest(ctx context.Context, project string) (Policy, error) {
	return scanPolicy(s.db.QueryRowContext(ctx, `
		SELECT id, project, description, owner, version, definition_hash,
		       users, roles, repositories, created_by, created_at
		FROM project_access_policies WHERE project = $1 ORDER BY version DESC LIMIT 1`, project))
}

func (s *PostgresStore) ListPolicies(ctx context.Context, project string, limit int) ([]Policy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project, description, owner, version, definition_hash,
		       users, roles, repositories, created_by, created_at
		FROM project_access_policies WHERE project = $1 ORDER BY version DESC LIMIT $2`, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list access policies: %w", err)
	}
	defer rows.Close()
	result := make([]Policy, 0)
	for rows.Next() {
		policy, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access policies: %w", err)
	}
	return result, nil
}

type rowScanner interface{ Scan(...any) error }

func scanPolicy(row rowScanner) (Policy, error) {
	var policy Policy
	var users, roles, repositories []byte
	if err := row.Scan(&policy.ID, &policy.Project, &policy.Description, &policy.Owner,
		&policy.Version, &policy.DefinitionHash, &users, &roles, &repositories,
		&policy.CreatedBy, &policy.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Policy{}, ErrPolicyNotFound
		}
		return Policy{}, fmt.Errorf("scan access policy: %w", err)
	}
	if err := json.Unmarshal(users, &policy.Users); err != nil {
		return Policy{}, fmt.Errorf("decode access policy users: %w", err)
	}
	if err := json.Unmarshal(roles, &policy.Roles); err != nil {
		return Policy{}, fmt.Errorf("decode access policy roles: %w", err)
	}
	if err := json.Unmarshal(repositories, &policy.Repositories); err != nil {
		return Policy{}, fmt.Errorf("decode access policy repositories: %w", err)
	}
	return policy, nil
}
