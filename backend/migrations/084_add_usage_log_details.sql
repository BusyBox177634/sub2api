CREATE TABLE IF NOT EXISTS usage_log_details (
    id BIGSERIAL PRIMARY KEY,
    usage_log_id BIGINT NOT NULL UNIQUE REFERENCES usage_logs(id) ON DELETE CASCADE,
    request_payload_json TEXT NULL,
    response_payload_json TEXT NULL,
    request_payload_bytes INTEGER NULL,
    response_payload_bytes INTEGER NULL,
    request_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_log_details_usage_log_id
    ON usage_log_details (usage_log_id);
