ALTER TABLE system_settings
    ADD COLUMN settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE system_settings
SET updated_at = now()
WHERE updated_at IS NULL;
