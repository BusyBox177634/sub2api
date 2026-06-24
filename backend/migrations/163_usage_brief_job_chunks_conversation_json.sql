ALTER TABLE usage_brief_job_chunks
    ADD COLUMN IF NOT EXISTS conversation_json JSONB NOT NULL DEFAULT '{}'::jsonb;
