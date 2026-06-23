ALTER TABLE ops_system_metrics
    ADD COLUMN IF NOT EXISTS disk_mounts_json JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN ops_system_metrics.disk_mounts_json IS 'Visible filesystem mount usage snapshots for ops dashboard disk capacity display.';
