package evaluation

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/0001_create_evaluations.sql
var evaluationSchema string

//go:embed migrations/0002_add_recoverable_runs.sql
var recoverableRunsMigration string

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open evaluation PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect evaluation PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, evaluationSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize evaluation schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, recoverableRunsMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize recoverable evaluation schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) CreateSuite(ctx context.Context, suite Suite) (Suite, error) {
	casesJSON, err := json.Marshal(suite.Cases)
	if err != nil {
		return Suite{}, fmt.Errorf("encode evaluation cases: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Suite{}, fmt.Errorf("begin evaluation suite creation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", suite.Name); err != nil {
		return Suite{}, fmt.Errorf("lock evaluation suite name: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM evaluation_suites
		WHERE name = $1`, suite.Name).Scan(&suite.Version); err != nil {
		return Suite{}, fmt.Errorf("allocate evaluation suite version: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO evaluation_suites (
			id, name, description, version, definition_hash, minimum_pass_rate,
			cases, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		suite.ID, suite.Name, suite.Description, suite.Version, suite.DefinitionHash,
		suite.MinimumPassRate, casesJSON, suite.CreatedBy, suite.CreatedAt,
	)
	if err != nil {
		return Suite{}, fmt.Errorf("create evaluation suite: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Suite{}, fmt.Errorf("commit evaluation suite: %w", err)
	}
	return suite, nil
}

func (s *PostgresStore) GetSuite(ctx context.Context, id string) (Suite, error) {
	return scanSuite(s.db.QueryRowContext(ctx, `
		SELECT id, name, description, version, definition_hash, minimum_pass_rate,
		       cases, created_by, created_at
		FROM evaluation_suites
		WHERE id = $1`, id))
}

func (s *PostgresStore) ListSuites(ctx context.Context, name string, limit int) ([]Suite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, version, definition_hash, minimum_pass_rate,
		       cases, created_by, created_at
		FROM evaluation_suites
		WHERE $1 = '' OR name = $1
		ORDER BY created_at DESC, version DESC
		LIMIT $2`, strings.TrimSpace(name), limit)
	if err != nil {
		return nil, fmt.Errorf("list evaluation suites: %w", err)
	}
	defer rows.Close()
	result := make([]Suite, 0)
	for rows.Next() {
		suite, err := scanSuite(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, suite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evaluation suites: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) CreateRun(ctx context.Context, run Run) error {
	if run.Attempt < 1 {
		run.Attempt = 1
	}
	reportJSON, err := json.Marshal(run.Report)
	if err != nil {
		return fmt.Errorf("encode evaluation report: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO evaluation_runs (
			id, suite_id, suite_name, suite_version, definition_hash,
			minimum_pass_rate, status, gate_status, error_code, triggered_by,
			report, created_at, started_at, completed_at, retry_of_run_id,
			attempt, cancel_requested, worker_id, lease_until
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
		          $15, $16, $17, $18, $19)`,
		run.ID, run.SuiteID, run.SuiteName, run.SuiteVersion, run.DefinitionHash,
		run.MinimumPassRate, run.Status, run.GateStatus, run.ErrorCode,
		run.TriggeredBy, reportJSON, run.CreatedAt, nullableTime(run.StartedAt),
		nullableTime(run.CompletedAt), run.RetryOfRunID, run.Attempt,
		run.CancelRequested, run.WorkerID, nullableTime(run.LeaseUntil),
	)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == "evaluation_runs_retry_of_unique_idx" {
			return ErrRunAlreadyRetried
		}
		return fmt.Errorf("create evaluation run: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateRun(ctx context.Context, run Run) error {
	reportJSON, err := json.Marshal(run.Report)
	if err != nil {
		return fmt.Errorf("encode evaluation report: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE evaluation_runs
		SET status = $2, gate_status = $3, error_code = $4, report = $5,
		    started_at = $6, completed_at = $7, retry_of_run_id = $8,
		    attempt = $9, cancel_requested = $10, worker_id = $11, lease_until = $12
		WHERE id = $1`,
		run.ID, run.Status, run.GateStatus, run.ErrorCode, reportJSON,
		nullableTime(run.StartedAt), nullableTime(run.CompletedAt), run.RetryOfRunID,
		run.Attempt, run.CancelRequested, run.WorkerID, nullableTime(run.LeaseUntil),
	)
	if err != nil {
		return fmt.Errorf("update evaluation run: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read evaluation run update count: %w", err)
	}
	if count == 0 {
		return ErrRunNotFound
	}
	return nil
}

func (s *PostgresStore) GetRun(ctx context.Context, id string) (Run, error) {
	return scanRun(s.db.QueryRowContext(ctx, `
		SELECT id, suite_id, suite_name, suite_version, definition_hash,
		       minimum_pass_rate, status, gate_status, error_code, triggered_by,
		       report, created_at, started_at, completed_at, retry_of_run_id,
		       attempt, cancel_requested, worker_id, lease_until
		FROM evaluation_runs
		WHERE id = $1`, id))
}

func (s *PostgresStore) ListRuns(ctx context.Context, suiteID string, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, suite_id, suite_name, suite_version, definition_hash,
		       minimum_pass_rate, status, gate_status, error_code, triggered_by,
		       report, created_at, started_at, completed_at, retry_of_run_id,
		       attempt, cancel_requested, worker_id, lease_until
		FROM evaluation_runs
		WHERE $1 = '' OR suite_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, strings.TrimSpace(suiteID), limit)
	if err != nil {
		return nil, fmt.Errorf("list evaluation runs: %w", err)
	}
	defer rows.Close()
	result := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evaluation runs: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) ListRunnable(ctx context.Context, now time.Time, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM evaluation_runs
		WHERE cancel_requested = FALSE
		  AND (status = 'pending' OR (status = 'running' AND (lease_until IS NULL OR lease_until < $1)))
		ORDER BY created_at
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list runnable evaluation runs: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan runnable evaluation run: %w", err)
		}
		result = append(result, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runnable evaluation runs: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) ClaimRun(ctx context.Context, id, workerID string, now, leaseUntil time.Time) (Run, bool, error) {
	run, err := scanRun(s.db.QueryRowContext(ctx, `
		UPDATE evaluation_runs
		SET status = 'running', worker_id = $2, lease_until = $4,
		    started_at = COALESCE(started_at, $3)
		WHERE id = $1 AND cancel_requested = FALSE
		  AND (status = 'pending' OR (status = 'running' AND (lease_until IS NULL OR lease_until < $3)))
		RETURNING id, suite_id, suite_name, suite_version, definition_hash,
		          minimum_pass_rate, status, gate_status, error_code, triggered_by,
		          report, created_at, started_at, completed_at, retry_of_run_id,
		          attempt, cancel_requested, worker_id, lease_until`,
		id, workerID, now, leaseUntil))
	if err != nil {
		if err == ErrRunNotFound {
			if _, getErr := s.GetRun(ctx, id); getErr != nil {
				return Run{}, false, getErr
			}
			return Run{}, false, nil
		}
		return Run{}, false, err
	}
	return run, true, nil
}

func (s *PostgresStore) RenewLease(ctx context.Context, id, workerID string, leaseUntil time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE evaluation_runs
		SET lease_until = $3
		WHERE id = $1 AND worker_id = $2 AND status = 'running'`, id, workerID, leaseUntil)
	if err != nil {
		return fmt.Errorf("renew evaluation run lease: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read renewed evaluation run count: %w", err)
	}
	if count == 0 {
		return ErrRunNotClaimed
	}
	return nil
}

func (s *PostgresStore) RequestCancel(ctx context.Context, id string, now time.Time) (Run, error) {
	return scanRun(s.db.QueryRowContext(ctx, `
		UPDATE evaluation_runs
		SET cancel_requested = CASE WHEN status IN ('pending', 'running') THEN TRUE ELSE cancel_requested END,
		    gate_status = CASE WHEN status IN ('pending', 'running') THEN 'canceled' ELSE gate_status END,
		    completed_at = CASE WHEN status IN ('pending', 'running') THEN $2 ELSE completed_at END,
		    worker_id = CASE WHEN status IN ('pending', 'running') THEN '' ELSE worker_id END,
		    lease_until = CASE WHEN status IN ('pending', 'running') THEN NULL ELSE lease_until END,
		    status = CASE WHEN status IN ('pending', 'running') THEN 'canceled' ELSE status END
		WHERE id = $1
		RETURNING id, suite_id, suite_name, suite_version, definition_hash,
		          minimum_pass_rate, status, gate_status, error_code, triggered_by,
		          report, created_at, started_at, completed_at, retry_of_run_id,
		          attempt, cancel_requested, worker_id, lease_until`, id, now))
}

func (s *PostgresStore) CompleteRun(ctx context.Context, run Run, workerID string) error {
	reportJSON, err := json.Marshal(run.Report)
	if err != nil {
		return fmt.Errorf("encode evaluation report: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE evaluation_runs
		SET status = $3, gate_status = $4, error_code = $5, report = $6,
		    cancel_requested = $7, completed_at = $8, worker_id = '', lease_until = NULL
		WHERE id = $1 AND worker_id = $2 AND status = 'running'`,
		run.ID, workerID, run.Status, run.GateStatus, run.ErrorCode, reportJSON,
		run.CancelRequested, nullableTime(run.CompletedAt))
	if err != nil {
		return fmt.Errorf("complete evaluation run: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read completed evaluation run count: %w", err)
	}
	if count == 0 {
		return ErrRunNotClaimed
	}
	return nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSuite(scanner rowScanner) (Suite, error) {
	var suite Suite
	var casesJSON []byte
	if err := scanner.Scan(
		&suite.ID, &suite.Name, &suite.Description, &suite.Version,
		&suite.DefinitionHash, &suite.MinimumPassRate, &casesJSON,
		&suite.CreatedBy, &suite.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return Suite{}, ErrSuiteNotFound
		}
		return Suite{}, fmt.Errorf("scan evaluation suite: %w", err)
	}
	if err := json.Unmarshal(casesJSON, &suite.Cases); err != nil {
		return Suite{}, fmt.Errorf("decode evaluation cases: %w", err)
	}
	return suite, nil
}

func scanRun(scanner rowScanner) (Run, error) {
	var run Run
	var reportJSON []byte
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	var leaseUntil sql.NullTime
	if err := scanner.Scan(
		&run.ID, &run.SuiteID, &run.SuiteName, &run.SuiteVersion,
		&run.DefinitionHash, &run.MinimumPassRate, &run.Status,
		&run.GateStatus, &run.ErrorCode, &run.TriggeredBy, &reportJSON,
		&run.CreatedAt, &startedAt, &completedAt, &run.RetryOfRunID,
		&run.Attempt, &run.CancelRequested, &run.WorkerID, &leaseUntil,
	); err != nil {
		if err == sql.ErrNoRows {
			return Run{}, ErrRunNotFound
		}
		return Run{}, fmt.Errorf("scan evaluation run: %w", err)
	}
	if err := json.Unmarshal(reportJSON, &run.Report); err != nil {
		return Run{}, fmt.Errorf("decode evaluation report: %w", err)
	}
	if startedAt.Valid {
		run.StartedAt = startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = completedAt.Time
	}
	if leaseUntil.Valid {
		run.LeaseUntil = leaseUntil.Time
	}
	return run, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
