CREATE TABLE IF NOT EXISTS repository_knowledge_sync_reports (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('completed', 'completed_with_errors')),
    repository TEXT NOT NULL,
    project TEXT NOT NULL,
    business_domain TEXT NOT NULL,
    classification TEXT NOT NULL CHECK (classification IN ('public', 'internal', 'restricted')),
    allowed_roles JSONB NOT NULL,
    head_commit TEXT NOT NULL,
    previous_head_commit TEXT NOT NULL,
    branch TEXT NOT NULL,
    full_resync BOOLEAN NOT NULL,
    commit_changes JSONB NOT NULL,
    scanned INTEGER NOT NULL CHECK (scanned >= 0),
    created INTEGER NOT NULL CHECK (created >= 0),
    updated INTEGER NOT NULL CHECK (updated >= 0),
    skipped INTEGER NOT NULL CHECK (skipped >= 0),
    deleted INTEGER NOT NULL CHECK (deleted >= 0),
    failed INTEGER NOT NULL CHECK (failed >= 0),
    sensitive_files_skipped INTEGER NOT NULL CHECK (sensitive_files_skipped >= 0),
    redaction_count INTEGER NOT NULL CHECK (redaction_count >= 0),
    files JSONB NOT NULL,
    synced_by TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS repository_knowledge_sync_project_time_idx
    ON repository_knowledge_sync_reports (project, started_at DESC);

CREATE INDEX IF NOT EXISTS repository_knowledge_sync_baseline_idx
    ON repository_knowledge_sync_reports (project, repository, completed_at DESC)
    WHERE status = 'completed';
