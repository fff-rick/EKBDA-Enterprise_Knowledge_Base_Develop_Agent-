CREATE TABLE IF NOT EXISTS workspace_validation_snapshots (
    id TEXT PRIMARY KEY,
    repository TEXT NOT NULL,
    project TEXT NOT NULL,
    technology TEXT NOT NULL,
    head_commit TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL DEFAULT '',
    dirty BOOLEAN NOT NULL,
    file_count INTEGER NOT NULL,
    tracked_count INTEGER NOT NULL,
    untracked_count INTEGER NOT NULL,
    binary_count INTEGER NOT NULL,
    changed_count INTEGER NOT NULL,
    changes JSONB NOT NULL,
    input_hash TEXT NOT NULL,
    standards_report_id TEXT NOT NULL,
    passed BOOLEAN NOT NULL,
    validated_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS workspace_validation_project_created_idx
    ON workspace_validation_snapshots (project, created_at DESC);

CREATE INDEX IF NOT EXISTS workspace_validation_report_idx
    ON workspace_validation_snapshots (standards_report_id);
