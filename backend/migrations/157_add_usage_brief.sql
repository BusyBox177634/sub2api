CREATE TABLE IF NOT EXISTS usage_brief_reports (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period_type VARCHAR(16) NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'queued',
    title TEXT NOT NULL DEFAULT '',
    content_md TEXT NOT NULL DEFAULT '',
    source_kind VARCHAR(16) NOT NULL DEFAULT 'production',
    is_admin_edited BOOLEAN NOT NULL DEFAULT FALSE,
    edited_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    generated_by_job_id BIGINT,
    input_usage_count INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    generated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_brief_reports_period_type_check CHECK (period_type IN ('daily', 'weekly', 'monthly')),
    CONSTRAINT usage_brief_reports_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
    CONSTRAINT usage_brief_reports_source_kind_check CHECK (source_kind IN ('production', 'test')),
    CONSTRAINT usage_brief_reports_period_check CHECK (period_end >= period_start),
    CONSTRAINT usage_brief_reports_unique_period UNIQUE (user_id, period_type, period_start, period_end)
);

CREATE TABLE IF NOT EXISTS usage_brief_jobs (
    id BIGSERIAL PRIMARY KEY,
    job_scope VARCHAR(16) NOT NULL,
    job_type VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'queued',
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    period_type VARCHAR(16),
    period_start DATE,
    period_end DATE,
    range_start TIMESTAMPTZ,
    range_end TIMESTAMPTZ,
    progress_current INTEGER NOT NULL DEFAULT 0,
    progress_total INTEGER NOT NULL DEFAULT 0,
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count INTEGER NOT NULL DEFAULT 0,
    report_id BIGINT REFERENCES usage_brief_reports(id) ON DELETE SET NULL,
    result_md TEXT,
    error_message TEXT,
    locked_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_brief_jobs_scope_check CHECK (job_scope IN ('production', 'test')),
    CONSTRAINT usage_brief_jobs_type_check CHECK (job_type IN ('daily', 'weekly', 'monthly', 'custom')),
    CONSTRAINT usage_brief_jobs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
    CONSTRAINT usage_brief_jobs_period_type_check CHECK (period_type IS NULL OR period_type IN ('daily', 'weekly', 'monthly')),
    CONSTRAINT usage_brief_jobs_period_check CHECK (period_start IS NULL OR period_end IS NULL OR period_end >= period_start),
    CONSTRAINT usage_brief_jobs_range_check CHECK (range_start IS NULL OR range_end IS NULL OR range_end > range_start)
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_brief_reports_generated_by_job_id_fkey'
          AND conrelid = 'usage_brief_reports'::regclass
    ) THEN
        ALTER TABLE usage_brief_reports
            ADD CONSTRAINT usage_brief_reports_generated_by_job_id_fkey
            FOREIGN KEY (generated_by_job_id)
            REFERENCES usage_brief_jobs(id)
            ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_usage_brief_reports_user_period
    ON usage_brief_reports (user_id, period_type, period_start DESC);

CREATE INDEX IF NOT EXISTS idx_usage_brief_reports_period
    ON usage_brief_reports (period_type, period_start DESC, status);

CREATE INDEX IF NOT EXISTS idx_usage_brief_jobs_scope_status
    ON usage_brief_jobs (job_scope, status, created_at);

CREATE INDEX IF NOT EXISTS idx_usage_brief_jobs_user_period
    ON usage_brief_jobs (user_id, period_type, period_start DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_brief_jobs_unique_active_production
    ON usage_brief_jobs (user_id, period_type, period_start, period_end)
    WHERE job_scope = 'production' AND status IN ('queued', 'running');
