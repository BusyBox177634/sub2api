ALTER TABLE usage_brief_jobs
    DROP CONSTRAINT IF EXISTS usage_brief_jobs_status_check;

ALTER TABLE usage_brief_jobs
    ADD CONSTRAINT usage_brief_jobs_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'paused'));
