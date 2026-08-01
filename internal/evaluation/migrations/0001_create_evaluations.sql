CREATE TABLE IF NOT EXISTS evaluation_suites (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL CHECK (version > 0),
    definition_hash TEXT NOT NULL,
    minimum_pass_rate DOUBLE PRECISION NOT NULL CHECK (minimum_pass_rate >= 0 AND minimum_pass_rate <= 1),
    cases JSONB NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (name, version)
);

CREATE INDEX IF NOT EXISTS evaluation_suites_name_version_idx
    ON evaluation_suites (name, version DESC);

CREATE TABLE IF NOT EXISTS evaluation_runs (
    id TEXT PRIMARY KEY,
    suite_id TEXT NOT NULL REFERENCES evaluation_suites(id),
    suite_name TEXT NOT NULL,
    suite_version INTEGER NOT NULL CHECK (suite_version > 0),
    definition_hash TEXT NOT NULL,
    minimum_pass_rate DOUBLE PRECISION NOT NULL CHECK (minimum_pass_rate >= 0 AND minimum_pass_rate <= 1),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    gate_status TEXT NOT NULL CHECK (gate_status IN ('pending', 'passed', 'failed', 'error')),
    error_code TEXT NOT NULL DEFAULT '',
    triggered_by TEXT NOT NULL,
    report JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS evaluation_runs_suite_created_at_idx
    ON evaluation_runs (suite_id, created_at DESC);

CREATE INDEX IF NOT EXISTS evaluation_runs_created_at_idx
    ON evaluation_runs (created_at DESC);
