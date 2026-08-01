CREATE TABLE IF NOT EXISTS agent_tasks (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('role_review','project_package')),
    step TEXT NOT NULL,
    project TEXT NOT NULL,
    repository TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('pending','running','completed','failed','canceled')),
    error_code TEXT NOT NULL DEFAULT '',
    input JSONB NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    retry_of_task_id TEXT NOT NULL DEFAULT '',
    attempt INTEGER NOT NULL CHECK (attempt BETWEEN 1 AND 3),
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    worker_id TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    triggered_by TEXT NOT NULL,
    retry_requested_by TEXT NOT NULL DEFAULT '',
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    quality JSONB NOT NULL DEFAULT '{"passed":false,"checks":[]}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS agent_tasks_retry_of_unique_idx
    ON agent_tasks (retry_of_task_id) WHERE retry_of_task_id <> '';

CREATE INDEX IF NOT EXISTS agent_tasks_runnable_idx
    ON agent_tasks (status, lease_until, created_at)
    WHERE status IN ('pending','running');

CREATE INDEX IF NOT EXISTS agent_tasks_project_history_idx
    ON agent_tasks (project, kind, status, created_at DESC);
