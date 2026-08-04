CREATE TABLE IF NOT EXISTS release_requests (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    status TEXT NOT NULL,
    revision INTEGER NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS release_requests_project_created_idx
    ON release_requests (project, created_at DESC);

CREATE INDEX IF NOT EXISTS release_requests_status_updated_idx
    ON release_requests (status, updated_at);

CREATE TABLE IF NOT EXISTS release_events (
    id TEXT PRIMARY KEY,
    release_id TEXT NOT NULL REFERENCES release_requests(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    actor TEXT NOT NULL,
    reason TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (release_id, sequence)
);

CREATE INDEX IF NOT EXISTS release_events_release_idx
    ON release_events (release_id, sequence);

CREATE TABLE IF NOT EXISTS release_provider_receipts (
    event_id TEXT PRIMARY KEY,
    release_id TEXT NOT NULL REFERENCES release_requests(id) ON DELETE CASCADE,
    payload_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
