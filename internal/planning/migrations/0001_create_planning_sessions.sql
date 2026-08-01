CREATE TABLE IF NOT EXISTS planning_sessions (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    repository TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('awaiting_clarification', 'awaiting_approval', 'approved', 'rejected')),
    revision INTEGER NOT NULL CHECK (revision > 0),
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS planning_sessions_project_created_idx
    ON planning_sessions (project, created_at DESC);

CREATE TABLE IF NOT EXISTS planning_session_events (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES planning_sessions(id),
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    event_type TEXT NOT NULL,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    actor TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (session_id, sequence)
);

CREATE INDEX IF NOT EXISTS planning_session_events_session_sequence_idx
    ON planning_session_events (session_id, sequence);

