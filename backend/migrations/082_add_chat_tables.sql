CREATE TABLE IF NOT EXISTS chat_conversations (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    api_key_id      BIGINT       NOT NULL REFERENCES api_keys (id) ON DELETE RESTRICT,
    title           VARCHAR(255) NOT NULL DEFAULT '',
    model           VARCHAR(255) NOT NULL,
    last_message_at TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_conversations_user_updated_at
    ON chat_conversations (user_id, updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_chat_conversations_user_api_key
    ON chat_conversations (user_id, api_key_id);

CREATE TABLE IF NOT EXISTS chat_messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT       NOT NULL REFERENCES chat_conversations (id) ON DELETE CASCADE,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            VARCHAR(20)  NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'completed',
    text            TEXT         NOT NULL DEFAULT '',
    model           VARCHAR(255) NOT NULL DEFAULT '',
    attachment_ids  JSONB        NOT NULL DEFAULT '[]'::jsonb,
    error_message   TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation_created_at
    ON chat_messages (conversation_id, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_chat_messages_user_created_at
    ON chat_messages (user_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS chat_attachments (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT       NOT NULL REFERENCES chat_conversations (id) ON DELETE CASCADE,
    message_id      BIGINT       NULL REFERENCES chat_messages (id) ON DELETE SET NULL,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind            VARCHAR(20)  NOT NULL,
    mime_type       VARCHAR(128) NOT NULL,
    original_name   VARCHAR(255) NOT NULL DEFAULT '',
    size_bytes      BIGINT       NOT NULL DEFAULT 0,
    storage_type    VARCHAR(20)  NOT NULL DEFAULT 'local',
    storage_path    TEXT         NOT NULL,
    sha256          VARCHAR(64)  NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_attachments_conversation_created_at
    ON chat_attachments (conversation_id, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_chat_attachments_message_id
    ON chat_attachments (message_id);

CREATE INDEX IF NOT EXISTS idx_chat_attachments_user_created_at
    ON chat_attachments (user_id, created_at DESC, id DESC);
