DO $migration$
BEGIN
    ALTER TABLE shopping_idempotency
        DROP CONSTRAINT shopping_idempotency_operation_check,
        ADD CONSTRAINT shopping_idempotency_operation_check CHECK (
            operation IN ('lists', 'share-imports')
            OR operation ~ '^(items|shares):[1-9][0-9]{0,18}$'
        );

    CREATE TABLE shopping_list_shares (
        id bigserial PRIMARY KEY CHECK (id > 0),
        creator_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        source_list_id bigint NOT NULL CHECK (source_list_id > 0),
        token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
        name text NOT NULL,
        preserve_marks boolean NOT NULL,
        created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
        CONSTRAINT shopping_list_shares_name_check CHECK (
            char_length(name) BETWEEN 1 AND 100
            AND btrim(name, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
            AND name !~ U&'[\0001-\001F\007F-\009F]'
        )
    );
    CREATE INDEX shopping_list_shares_creator_id_id_idx
        ON shopping_list_shares (creator_id, id);

    CREATE TABLE shopping_list_share_items (
        share_id bigint NOT NULL REFERENCES shopping_list_shares(id) ON DELETE CASCADE,
        position bigint NOT NULL CHECK (position > 0),
        name text NOT NULL,
        quantity numeric,
        unit text NOT NULL DEFAULT '',
        is_marked boolean NOT NULL DEFAULT false,
        PRIMARY KEY (share_id, position),
        CONSTRAINT shopping_list_share_items_name_check CHECK (
            char_length(name) BETWEEN 1 AND 200
            AND btrim(name, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
            AND name !~ U&'[\0001-\001F\007F-\009F]'
        ),
        CONSTRAINT shopping_list_share_items_unit_check CHECK (
            char_length(unit) <= 32 AND unit !~ U&'[\0001-\001F\007F-\009F]'
        ),
        CONSTRAINT shopping_list_share_items_quantity_check CHECK (
            quantity IS NULL OR (
                quantity BETWEEN 0.001 AND 999999999.999
                AND quantity = trunc(quantity, 3)
            )
        )
    );
END
$migration$;
