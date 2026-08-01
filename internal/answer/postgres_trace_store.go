package answer

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/0001_create_answer_traces.sql
var traceSchema string

//go:embed migrations/0002_add_trace_costs.sql
var traceCostMigration string

type PostgresTraceStore struct {
	db *sql.DB
}

func NewPostgresTraceStore(ctx context.Context, dsn string) (*PostgresTraceStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open answer trace PostgreSQL store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect answer trace PostgreSQL store: %w", err)
	}
	if _, err := db.ExecContext(ctx, traceSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize answer trace schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, traceCostMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize answer trace cost schema: %w", err)
	}
	return &PostgresTraceStore{db: db}, nil
}

func (s *PostgresTraceStore) Save(ctx context.Context, trace Trace) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO answer_traces (
			id, user_id, query_hash, query_length, project, provider, status,
			error_code, refused, refusal_reason, evidence_count, citation_count,
			prompt_tokens, completion_tokens, total_tokens,
			input_usd_per_million_tokens, output_usd_per_million_tokens,
			prompt_cost_usd, completion_cost_usd, total_cost_usd,
			duration_ms, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22
		)`,
		trace.ID, trace.UserID, trace.QueryHash, trace.QueryLength, trace.Project,
		trace.Provider, trace.Status, trace.ErrorCode, trace.Refused,
		trace.RefusalReason, trace.EvidenceCount, trace.CitationCount,
		trace.PromptTokens, trace.CompletionTokens, trace.TotalTokens,
		trace.InputRateUSD, trace.OutputRateUSD, trace.PromptCostUSD,
		trace.CompletionCostUSD, trace.TotalCostUSD, trace.DurationMS, trace.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save answer trace: %w", err)
	}
	return nil
}

func (s *PostgresTraceStore) Get(ctx context.Context, id string) (Trace, error) {
	var trace Trace
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, query_hash, query_length, project, provider, status,
		       error_code, refused, refusal_reason, evidence_count, citation_count,
		       prompt_tokens, completion_tokens, total_tokens,
		       input_usd_per_million_tokens, output_usd_per_million_tokens,
		       prompt_cost_usd, completion_cost_usd, total_cost_usd,
		       duration_ms, created_at
		FROM answer_traces
		WHERE id = $1`, id).Scan(
		&trace.ID, &trace.UserID, &trace.QueryHash, &trace.QueryLength,
		&trace.Project, &trace.Provider, &trace.Status, &trace.ErrorCode,
		&trace.Refused, &trace.RefusalReason, &trace.EvidenceCount,
		&trace.CitationCount, &trace.PromptTokens, &trace.CompletionTokens,
		&trace.TotalTokens, &trace.InputRateUSD, &trace.OutputRateUSD,
		&trace.PromptCostUSD, &trace.CompletionCostUSD, &trace.TotalCostUSD,
		&trace.DurationMS, &trace.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Trace{}, ErrTraceNotFound
		}
		return Trace{}, fmt.Errorf("get answer trace: %w", err)
	}
	return trace, nil
}

func (s *PostgresTraceStore) Metrics(ctx context.Context, project string) (Metrics, error) {
	project = strings.TrimSpace(project)
	metrics := Metrics{Project: project, ByProvider: make(map[string]int)}
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'succeeded'),
		       COUNT(*) FILTER (WHERE status = 'failed'),
		       COUNT(*) FILTER (WHERE refused),
		       COALESCE(AVG(duration_ms), 0),
		       COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(total_cost_usd), 0)
		FROM answer_traces
		WHERE $1 = '' OR project = $1`, project).Scan(
		&metrics.Total, &metrics.Succeeded, &metrics.Errors, &metrics.Refused,
		&metrics.AverageDuration, &metrics.TotalTokens, &metrics.TotalCostUSD,
	)
	if err != nil {
		return Metrics{}, fmt.Errorf("aggregate answer traces: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, COUNT(*)
		FROM answer_traces
		WHERE $1 = '' OR project = $1
		GROUP BY provider
		ORDER BY provider`, project)
	if err != nil {
		return Metrics{}, fmt.Errorf("aggregate answer traces by provider: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var provider string
		var count int
		if err := rows.Scan(&provider, &count); err != nil {
			return Metrics{}, fmt.Errorf("scan answer provider metric: %w", err)
		}
		metrics.ByProvider[provider] = count
	}
	if err := rows.Err(); err != nil {
		return Metrics{}, fmt.Errorf("iterate answer provider metrics: %w", err)
	}
	return metrics, nil
}

func (s *PostgresTraceStore) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM answer_traces WHERE created_at < $1", before)
	if err != nil {
		return 0, fmt.Errorf("delete expired answer traces: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted answer trace count: %w", err)
	}
	return deleted, nil
}

func (s *PostgresTraceStore) Close() error {
	return s.db.Close()
}
