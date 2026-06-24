ALTER TABLE usage_brief_jobs
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_usage_brief_jobs_visible_scope_status
    ON usage_brief_jobs (job_scope, status, created_at)
    WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_usage_brief_jobs_unique_active_production;

CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_brief_jobs_unique_active_production
    ON usage_brief_jobs (user_id, period_type, period_start, period_end)
    WHERE job_scope = 'production'
      AND status IN ('queued', 'running')
      AND deleted_at IS NULL;
