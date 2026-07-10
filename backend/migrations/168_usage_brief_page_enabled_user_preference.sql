ALTER TABLE users
    ADD COLUMN IF NOT EXISTS usage_brief_page_enabled BOOLEAN;

UPDATE users
SET usage_brief_page_enabled = TRUE
WHERE usage_brief_page_enabled IS NULL;

ALTER TABLE users
    ALTER COLUMN usage_brief_page_enabled SET DEFAULT TRUE,
    ALTER COLUMN usage_brief_page_enabled SET NOT NULL;
