ALTER TABLE usage_brief_jobs
    ADD COLUMN IF NOT EXISTS email_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS email_sent_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS email_error_message TEXT,
    ADD COLUMN IF NOT EXISTS email_attempt_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE usage_brief_jobs
    DROP CONSTRAINT IF EXISTS usage_brief_jobs_email_status_check;

ALTER TABLE usage_brief_jobs
    ADD CONSTRAINT usage_brief_jobs_email_status_check
    CHECK (email_status IN ('pending', 'sent', 'failed', 'skipped'));

CREATE INDEX IF NOT EXISTS idx_usage_brief_jobs_email_status
    ON usage_brief_jobs (email_status, finished_at DESC)
    WHERE deleted_at IS NULL;
