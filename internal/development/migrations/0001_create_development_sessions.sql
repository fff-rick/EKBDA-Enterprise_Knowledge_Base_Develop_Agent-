CREATE TABLE IF NOT EXISTS development_sessions (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    repository TEXT NOT NULL,
    status TEXT NOT NULL,
    revision INTEGER NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS development_sessions_project_created_idx
    ON development_sessions (project, created_at DESC);

CREATE INDEX IF NOT EXISTS development_sessions_status_updated_idx
    ON development_sessions (status, updated_at);

CREATE TABLE IF NOT EXISTS development_session_events (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES development_sessions(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    actor TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (session_id, sequence)
);

CREATE INDEX IF NOT EXISTS development_session_events_session_idx
    ON development_session_events (session_id, sequence);
