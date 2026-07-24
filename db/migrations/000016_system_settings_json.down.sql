ALTER TABLE system_settings
    DROP COLUMN IF EXISTS settings_json,
    DROP COLUMN IF EXISTS updated_at;
