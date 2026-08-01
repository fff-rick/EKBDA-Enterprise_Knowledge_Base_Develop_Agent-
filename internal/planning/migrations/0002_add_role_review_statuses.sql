ALTER TABLE planning_sessions
    DROP CONSTRAINT IF EXISTS planning_sessions_status_check;

ALTER TABLE planning_sessions
    ADD CONSTRAINT planning_sessions_status_check
    CHECK (status IN (
        'awaiting_clarification',
        'awaiting_role_review',
        'awaiting_resolution',
        'awaiting_approval',
        'approved',
        'rejected'
    ));
