package agenttask

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

//go:embed migrations/0001_create_agent_tasks.sql
var agentTaskSchema string

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open agent task PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect agent task PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, agentTaskSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize agent task schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Create(ctx context.Context, task Task) error {
	quality, err := json.Marshal(task.Quality)
	if err != nil {
		return fmt.Errorf("encode agent task quality: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agent_tasks (
			id, kind, step, project, repository, status, error_code, input, resource_id,
			retry_of_task_id, attempt, cancel_requested, worker_id, lease_until, triggered_by, retry_requested_by,
			prompt_tokens, completion_tokens, total_tokens, cost_usd, quality,
			created_at, started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		task.ID, task.Kind, task.Step, task.Project, task.Repository, task.Status, task.ErrorCode,
		[]byte(task.Input), task.ResourceID, task.RetryOfTaskID, task.Attempt, task.CancelRequested,
		task.WorkerID, nullableTime(task.LeaseUntil), task.TriggeredBy, task.RetryRequestedBy, task.Usage.PromptTokens,
		task.Usage.CompletionTokens, task.Usage.TotalTokens, task.Usage.CostUSD, quality,
		task.CreatedAt, nullableTime(task.StartedAt), nullableTime(task.CompletedAt))
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.ConstraintName == "agent_tasks_retry_of_unique_idx" {
			return ErrTaskAlreadyRetried
		}
		return fmt.Errorf("create agent task: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Task, error) {
	return scanTask(s.db.QueryRowContext(ctx, taskSelect+` WHERE id=$1`, id))
}

func (s *PostgresStore) List(ctx context.Context, project, kind, status string, limit int) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, taskSelect+`
		WHERE project=$1 AND ($2='' OR kind=$2) AND ($3='' OR status=$3)
		ORDER BY created_at DESC LIMIT $4`, project, kind, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list agent tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *PostgresStore) ListRunnable(ctx context.Context, now time.Time, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM agent_tasks
		WHERE cancel_requested=FALSE
		  AND (status='pending' OR (status='running' AND (lease_until IS NULL OR lease_until < $1)))
		ORDER BY created_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list runnable agent tasks: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan runnable agent task: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (s *PostgresStore) Claim(ctx context.Context, id, workerID string, now, leaseUntil time.Time) (Task, bool, error) {
	task, err := scanTask(s.db.QueryRowContext(ctx, `
		UPDATE agent_tasks
		SET status='running', worker_id=$2, lease_until=$4,
		    started_at=COALESCE(started_at,$3)
		WHERE id=$1 AND cancel_requested=FALSE
		  AND (status='pending' OR (status='running' AND (lease_until IS NULL OR lease_until < $3)))
		RETURNING id, kind, step, project, repository, status, error_code, input, resource_id,
		          retry_of_task_id, attempt, cancel_requested, worker_id, lease_until, triggered_by, retry_requested_by,
		          prompt_tokens, completion_tokens, total_tokens, cost_usd, quality,
		          created_at, started_at, completed_at`, id, workerID, now, leaseUntil))
	if errors.Is(err, ErrTaskNotFound) {
		if _, getErr := s.Get(ctx, id); getErr != nil {
			return Task{}, false, getErr
		}
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, err
	}
	return task, true, nil
}

func (s *PostgresStore) RenewLease(ctx context.Context, id, workerID string, leaseUntil time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE agent_tasks SET lease_until=$3 WHERE id=$1 AND worker_id=$2 AND status='running'`, id, workerID, leaseUntil)
	if err != nil {
		return fmt.Errorf("renew agent task lease: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrTaskNotClaimed
	}
	return nil
}

func (s *PostgresStore) RequestCancel(ctx context.Context, id string, now time.Time) (Task, error) {
	return scanTask(s.db.QueryRowContext(ctx, `
		UPDATE agent_tasks
		SET cancel_requested=CASE WHEN status IN ('pending','running') THEN TRUE ELSE cancel_requested END,
		    status=CASE WHEN status='pending' THEN 'canceled' ELSE status END,
		    completed_at=CASE WHEN status='pending' THEN $2 ELSE completed_at END,
		    worker_id=CASE WHEN status='pending' THEN '' ELSE worker_id END,
		    lease_until=CASE WHEN status='pending' THEN NULL ELSE lease_until END
		WHERE id=$1
		RETURNING id, kind, step, project, repository, status, error_code, input, resource_id,
		          retry_of_task_id, attempt, cancel_requested, worker_id, lease_until, triggered_by, retry_requested_by,
		          prompt_tokens, completion_tokens, total_tokens, cost_usd, quality,
		          created_at, started_at, completed_at`, id, now))
}

func (s *PostgresStore) Complete(ctx context.Context, task Task, workerID string) error {
	quality, err := json.Marshal(task.Quality)
	if err != nil {
		return fmt.Errorf("encode completed agent task quality: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_tasks
		SET status=$3, error_code=$4, resource_id=$5, cancel_requested=$6,
		    prompt_tokens=$7, completion_tokens=$8, total_tokens=$9, cost_usd=$10,
		    quality=$11, completed_at=$12, worker_id='', lease_until=NULL
		WHERE id=$1 AND worker_id=$2 AND status='running'`,
		task.ID, workerID, task.Status, task.ErrorCode, task.ResourceID, task.CancelRequested,
		task.Usage.PromptTokens, task.Usage.CompletionTokens, task.Usage.TotalTokens, task.Usage.CostUSD,
		quality, nullableTime(task.CompletedAt))
	if err != nil {
		return fmt.Errorf("complete agent task: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrTaskNotClaimed
	}
	return nil
}

const taskSelect = `SELECT id, kind, step, project, repository, status, error_code, input, resource_id,
       retry_of_task_id, attempt, cancel_requested, worker_id, lease_until, triggered_by, retry_requested_by,
       prompt_tokens, completion_tokens, total_tokens, cost_usd, quality,
       created_at, started_at, completed_at FROM agent_tasks`

type rowScanner interface{ Scan(...any) error }

func scanTask(row rowScanner) (Task, error) {
	var task Task
	var input, quality []byte
	var leaseUntil, startedAt, completedAt sql.NullTime
	err := row.Scan(
		&task.ID, &task.Kind, &task.Step, &task.Project, &task.Repository, &task.Status,
		&task.ErrorCode, &input, &task.ResourceID, &task.RetryOfTaskID, &task.Attempt,
		&task.CancelRequested, &task.WorkerID, &leaseUntil, &task.TriggeredBy, &task.RetryRequestedBy,
		&task.Usage.PromptTokens, &task.Usage.CompletionTokens, &task.Usage.TotalTokens,
		&task.Usage.CostUSD, &quality, &task.CreatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("scan agent task: %w", err)
	}
	task.Input = append([]byte(nil), input...)
	if err := json.Unmarshal(quality, &task.Quality); err != nil {
		return Task{}, fmt.Errorf("decode agent task quality: %w", err)
	}
	if leaseUntil.Valid {
		task.LeaseUntil = leaseUntil.Time
	}
	if startedAt.Valid {
		task.StartedAt = startedAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = completedAt.Time
	}
	return task, nil
}

type rowsScanner interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanTasks(rows rowsScanner) ([]Task, error) {
	result := make([]Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent tasks: %w", err)
	}
	return result, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
