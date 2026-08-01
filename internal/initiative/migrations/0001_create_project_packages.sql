CREATE TABLE IF NOT EXISTS project_packages (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    repository TEXT NOT NULL,
    name TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    planning_session_id TEXT NOT NULL REFERENCES planning_sessions(id),
    definition_hash TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (project, name, version)
);

CREATE INDEX IF NOT EXISTS project_packages_project_name_version_idx
    ON project_packages (project, name, version DESC);

CREATE INDEX IF NOT EXISTS project_packages_planning_session_idx
    ON project_packages (planning_session_id);
