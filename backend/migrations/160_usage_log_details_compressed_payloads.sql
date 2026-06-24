ALTER TABLE usage_log_details
    ADD COLUMN IF NOT EXISTS compressed_request_payload_json TEXT NULL,
    ADD COLUMN IF NOT EXISTS compressed_response_payload_json TEXT NULL,
    ADD COLUMN IF NOT EXISTS full_payloads_cleaned_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_usage_log_details_full_payload_cleanup
    ON usage_log_details (created_at, usage_log_id)
    WHERE request_payload_json IS NOT NULL OR response_payload_json IS NOT NULL;
