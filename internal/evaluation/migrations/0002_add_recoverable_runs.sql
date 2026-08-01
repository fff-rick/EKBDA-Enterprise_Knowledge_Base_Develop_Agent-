ALTER TABLE evaluation_runs
    DROP CONSTRAINT IF EXISTS evaluation_runs_status_check,
    DROP CONSTRAINT IF EXISTS evaluation_runs_gate_status_check;

ALTER TABLE evaluation_runs
    ADD CONSTRAINT evaluation_runs_status_check
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'canceled')),
    ADD CONSTRAINT evaluation_runs_gate_status_check
        CHECK (gate_status IN ('pending', 'passed', 'failed', 'error', 'canceled')),
    ADD COLUMN IF NOT EXISTS retry_of_run_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt > 0),
    ADD COLUMN IF NOT EXISTS cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS worker_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS evaluation_runs_runnable_idx
    ON evaluation_runs (status, lease_until, created_at)
    WHERE status IN ('pending', 'running') AND cancel_requested = FALSE;

CREATE UNIQUE INDEX IF NOT EXISTS evaluation_runs_retry_of_unique_idx
    ON evaluation_runs (retry_of_run_id)
    WHERE retry_of_run_id <> '';
