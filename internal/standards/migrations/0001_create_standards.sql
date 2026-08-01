CREATE TABLE IF NOT EXISTS standard_packages (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL CHECK (scope IN ('common', 'technology', 'project')),
    selector TEXT NOT NULL DEFAULT '',
    owner TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    definition_hash TEXT NOT NULL,
    rules JSONB NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (scope, selector, name, version)
);

CREATE INDEX IF NOT EXISTS standard_packages_lookup_idx
    ON standard_packages (scope, selector, name, version DESC);

CREATE TABLE IF NOT EXISTS standard_validation_reports (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    technology TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    passed BOOLEAN NOT NULL,
    rule_count INTEGER NOT NULL,
    violation_count INTEGER NOT NULL,
    blocking_count INTEGER NOT NULL,
    packages JSONB NOT NULL,
    violations JSONB NOT NULL,
    validated_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS standard_validation_reports_project_created_idx
    ON standard_validation_reports (project, created_at DESC);
