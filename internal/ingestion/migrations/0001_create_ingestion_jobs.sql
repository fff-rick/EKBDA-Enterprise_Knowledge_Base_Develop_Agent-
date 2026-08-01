CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    root TEXT NOT NULL,
    project TEXT NOT NULL,
    scanned INTEGER NOT NULL DEFAULT 0,
    created INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    skipped INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS ingestion_job_files (
    job_id TEXT NOT NULL REFERENCES ingestion_jobs(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    action TEXT NOT NULL DEFAULT '',
    document_id TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (job_id, path)
);

CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_started_at
    ON ingestion_jobs (started_at DESC);
