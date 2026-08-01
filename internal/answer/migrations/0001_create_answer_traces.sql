CREATE TABLE IF NOT EXISTS answer_traces (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    query_length INTEGER NOT NULL CHECK (query_length >= 0),
    project TEXT NOT NULL,
    provider TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed')),
    error_code TEXT NOT NULL DEFAULT '',
    refused BOOLEAN NOT NULL DEFAULT FALSE,
    refusal_reason TEXT NOT NULL DEFAULT '',
    evidence_count INTEGER NOT NULL DEFAULT 0 CHECK (evidence_count >= 0),
    citation_count INTEGER NOT NULL DEFAULT 0 CHECK (citation_count >= 0),
    prompt_tokens INTEGER NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens INTEGER NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    total_tokens INTEGER NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS answer_traces_created_at_idx
    ON answer_traces (created_at DESC);

CREATE INDEX IF NOT EXISTS answer_traces_project_created_at_idx
    ON answer_traces (project, created_at DESC);
