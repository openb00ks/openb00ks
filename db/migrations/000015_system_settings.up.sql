CREATE TABLE system_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    setup_complete BOOLEAN NOT NULL DEFAULT false,
    setup_completed_at TIMESTAMPTZ
);

INSERT INTO system_settings (id, setup_complete)
VALUES (1, false)
ON CONFLICT (id) DO NOTHING;
