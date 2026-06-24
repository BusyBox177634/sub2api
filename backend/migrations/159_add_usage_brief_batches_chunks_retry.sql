ALTER TABLE usage_brief_reports
    ALTER COLUMN input_tokens TYPE BIGINT,
    ALTER COLUMN output_tokens TYPE BIGINT;

CREATE TABLE IF NOT EXISTS usage_brief_batches (
    id BIGSERIAL PRIMARY KEY,
    batch_scope VARCHAR(16) NOT NULL,
    trigger_kind VARCHAR(16) NOT NULL DEFAULT 'manual',
    status VARCHAR(16) NOT NULL DEFAULT 'queued',
    title TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    period_type VARCHAR(16),
    period_start DATE,
    period_end DATE,
    range_start TIMESTAMPTZ,
    range_end TIMESTAMPTZ,
    job_count INTEGER NOT NULL DEFAULT 0,
    succeeded_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    canceled_count INTEGER NOT NULL DEFAULT 0,
    running_count INTEGER NOT NULL DEFAULT 0,
    queued_count INTEGER NOT NULL DEFAULT 0,
    retrying_count INTEGER NOT NULL DEFAULT 0,
    token_estimated_total BIGINT NOT NULL DEFAULT 0,
    token_estimated_processed BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    paused_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_brief_batches_scope_check CHECK (batch_scope IN ('production', 'test')),
    CONSTRAINT usage_brief_batches_trigger_check CHECK (trigger_kind IN ('manual', 'auto')),
    CONSTRAINT usage_brief_batches_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'paused', 'partial')),
    CONSTRAINT usage_brief_batches_period_type_check CHECK (period_type IS NULL OR period_type IN ('daily', 'weekly', 'monthly')),
    CONSTRAINT usage_brief_batches_period_check CHECK (period_start IS NULL OR period_end IS NULL OR period_end >= period_start),
    CONSTRAINT usage_brief_batches_range_check CHECK (range_start IS NULL OR range_end IS NULL OR range_end > range_start)
);

ALTER TABLE usage_brief_jobs
    ADD COLUMN IF NOT EXISTS batch_id BIGINT REFERENCES usage_brief_batches(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS stage VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS chunk_current INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS chunk_total INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS token_estimated_total BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS token_estimated_processed BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS input_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS output_tokens BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS usage_brief_job_chunks (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES usage_brief_jobs(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    chunk_type VARCHAR(16) NOT NULL DEFAULT 'source',
    status VARCHAR(16) NOT NULL DEFAULT 'queued',
    token_estimated BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    content_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    summary_md TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_brief_job_chunks_type_check CHECK (chunk_type IN ('source', 'merge')),
    CONSTRAINT usage_brief_job_chunks_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
    CONSTRAINT usage_brief_job_chunks_unique UNIQUE (job_id, chunk_index, chunk_type)
);

CREATE INDEX IF NOT EXISTS idx_usage_brief_batches_visible_scope_status
    ON usage_brief_batches (batch_scope, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_usage_brief_jobs_batch_status
    ON usage_brief_jobs (batch_id, status, id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_usage_brief_jobs_retry
    ON usage_brief_jobs (status, next_retry_at, created_at)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_usage_brief_job_chunks_job
    ON usage_brief_job_chunks (job_id, chunk_type, chunk_index);
