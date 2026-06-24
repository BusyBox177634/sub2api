CREATE TABLE IF NOT EXISTS usage_brief_job_conversations (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES usage_brief_jobs(id) ON DELETE CASCADE,
    conversation_index INTEGER NOT NULL,
    usage_log_id BIGINT NOT NULL DEFAULT 0,
    covered_usage_log_ids BIGINT[] NOT NULL DEFAULT '{}'::BIGINT[],
    covered_request_count INTEGER NOT NULL DEFAULT 1,
    token_estimated BIGINT NOT NULL DEFAULT 0,
    conversation_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_brief_job_conversations_unique UNIQUE (job_id, conversation_index)
);

CREATE INDEX IF NOT EXISTS idx_usage_brief_job_conversations_job
    ON usage_brief_job_conversations (job_id, conversation_index);

INSERT INTO usage_brief_job_conversations (
    job_id,
    conversation_index,
    usage_log_id,
    covered_usage_log_ids,
    covered_request_count,
    token_estimated,
    conversation_json,
    created_at,
    updated_at
)
SELECT
    c.job_id,
    ROW_NUMBER() OVER (PARTITION BY c.job_id ORDER BY c.chunk_index, r.ordinality)::INTEGER AS conversation_index,
    COALESCE((r.record->>'id')::BIGINT, 0) AS usage_log_id,
    CASE
        WHEN jsonb_typeof(r.record->'covered_usage_log_ids') = 'array' THEN ARRAY(
            SELECT jsonb_array_elements_text(r.record->'covered_usage_log_ids')::BIGINT
        )
        WHEN r.record ? 'id' THEN ARRAY[(r.record->>'id')::BIGINT]
        ELSE '{}'::BIGINT[]
    END AS covered_usage_log_ids,
    COALESCE((r.record->>'covered_request_count')::INTEGER, 1) AS covered_request_count,
    GREATEST(1, CEIL(length(COALESCE((r.record->>'messages_json'), jsonb_build_object('messages', COALESCE(r.record->'messages', '[]'::jsonb))::TEXT))::NUMERIC / 4))::BIGINT AS token_estimated,
    jsonb_build_object('messages', COALESCE(r.record->'messages', '[]'::jsonb)) AS conversation_json,
    c.created_at,
    NOW()
FROM usage_brief_job_chunks c
CROSS JOIN LATERAL jsonb_array_elements(c.conversation_json->'records') WITH ORDINALITY AS r(record, ordinality)
WHERE c.chunk_type = 'source'
  AND jsonb_typeof(c.conversation_json->'records') = 'array'
ON CONFLICT (job_id, conversation_index) DO NOTHING;
