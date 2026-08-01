CREATE TABLE IF NOT EXISTS project_access_policies (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    description TEXT NOT NULL,
    owner TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    definition_hash TEXT NOT NULL,
    users JSONB NOT NULL,
    roles JSONB NOT NULL,
    repositories JSONB NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (project, version)
);

CREATE INDEX IF NOT EXISTS project_access_policies_project_version_idx
    ON project_access_policies (project, version DESC);

