CREATE TABLE IF NOT EXISTS project_package_artifact_reviews (
    id TEXT PRIMARY KEY,
    package_id TEXT NOT NULL REFERENCES project_packages(id),
    artifact_type TEXT NOT NULL CHECK (artifact_type IN ('prd','architecture','api','test','deployment','monitoring','risk')),
    package_hash TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    decision TEXT NOT NULL CHECK (decision IN ('approve','request_changes')),
    comment TEXT NOT NULL,
    reviewed_by TEXT NOT NULL,
    reviewed_at TIMESTAMPTZ NOT NULL,
    UNIQUE (package_id, artifact_type, sequence)
);

CREATE INDEX IF NOT EXISTS project_package_artifact_reviews_lookup_idx
    ON project_package_artifact_reviews (package_id, artifact_type, sequence DESC);
