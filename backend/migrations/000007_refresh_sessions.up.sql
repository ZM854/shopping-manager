-- Старый формат refresh отзывается: следующий вход создаст отдельную сессию.
-- Аккаунты и пользовательские данные сохраняются.
DELETE FROM refresh_tokens;

ALTER TABLE refresh_tokens
    DROP CONSTRAINT refresh_tokens_user_id_key,
    ADD COLUMN session_id UUID NOT NULL UNIQUE,
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    ADD COLUMN expires_at TIMESTAMPTZ NOT NULL,
    ADD COLUMN last_used_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    ADD CONSTRAINT refresh_tokens_expiry_check CHECK (expires_at > created_at),
    ADD CONSTRAINT refresh_tokens_hash_check CHECK (token_hash ~ '^[0-9a-f]{64}$');

CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);
CREATE INDEX refresh_tokens_expires_at_idx ON refresh_tokens (expires_at);
