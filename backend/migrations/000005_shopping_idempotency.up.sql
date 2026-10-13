CREATE TABLE shopping_idempotency (
    owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    operation text NOT NULL CHECK (operation = 'lists' OR operation ~ '^items:[1-9][0-9]{0,18}$'),
    key text NOT NULL CHECK (key ~ '^[A-Za-z0-9_-]{16,128}$'),
    request_hash text NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
    resource_id bigint,
    response jsonb,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (owner_id, operation, key),
    CHECK ((resource_id IS NULL) = (response IS NULL)),
    CHECK (resource_id IS NULL OR resource_id > 0)
);
CREATE INDEX shopping_idempotency_expires_at_idx ON shopping_idempotency (expires_at);
