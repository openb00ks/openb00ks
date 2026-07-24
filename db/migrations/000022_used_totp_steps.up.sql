CREATE TABLE used_totp_steps (
    user_id UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    step    BIGINT  NOT NULL,
    used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, step)
);
