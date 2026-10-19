-- Old activation links are intentionally invalidated; accounts remain intact.
ALTER TABLE users DROP COLUMN activation_token;
ALTER TABLE users
    ADD COLUMN activation_token_hash TEXT UNIQUE,
    ADD COLUMN activation_expires_at TIMESTAMPTZ,
    ADD COLUMN activation_next_send_at TIMESTAMPTZ,
    ADD CONSTRAINT users_activation_state CHECK (
        (activation_token_hash IS NULL AND activation_expires_at IS NULL)
        OR (
            NOT is_email_verified
            AND activation_token_hash IS NOT NULL
            AND activation_token_hash ~ '^[0-9a-f]{64}$'
            AND activation_expires_at IS NOT NULL
            AND activation_next_send_at IS NOT NULL
        )
    );
