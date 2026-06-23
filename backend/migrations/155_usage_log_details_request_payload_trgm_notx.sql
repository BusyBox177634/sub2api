CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_log_details_request_payload_trgm
    ON usage_log_details
    USING gin (request_payload_json gin_trgm_ops);
