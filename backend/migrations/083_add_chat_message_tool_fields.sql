ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS tool_events_json JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS citations_json JSONB NOT NULL DEFAULT '[]'::jsonb;
