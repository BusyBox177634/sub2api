ALTER TABLE usage_brief_jobs
    DROP CONSTRAINT IF EXISTS usage_brief_jobs_status_check;

ALTER TABLE usage_brief_jobs
    ADD CONSTRAINT usage_brief_jobs_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'paused', 'partial'));

ALTER TABLE usage_brief_reports
    DROP CONSTRAINT IF EXISTS usage_brief_reports_status_check;

ALTER TABLE usage_brief_reports
    ADD CONSTRAINT usage_brief_reports_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'partial'));

ALTER TABLE usage_brief_job_chunks
    ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error_at TIMESTAMPTZ;
