DO $migration$
BEGIN
    CREATE TABLE recipes (
        id bigserial PRIMARY KEY CHECK (id > 0),
        owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        title text NOT NULL,
        cooking_time_minutes integer NOT NULL,
        category_code text NOT NULL,
        created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
        CONSTRAINT recipes_title_check CHECK (
            char_length(title) BETWEEN 1 AND 200
            AND btrim(title, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
            AND title !~ U&'[\0001-\001F\007F-\009F]'
        ),
        CONSTRAINT recipes_cooking_time_check CHECK (
            cooking_time_minutes BETWEEN 5 AND 1440
            AND cooking_time_minutes % 5 = 0
        ),
        CONSTRAINT recipes_category_check CHECK (
            category_code IN (
                'soups', 'salads', 'starters', 'main-dishes', 'side-dishes',
                'baking', 'desserts', 'sauces', 'drinks', 'other'
            )
        )
    );
    CREATE INDEX recipes_owner_id_id_idx ON recipes (owner_id, id);

    CREATE TABLE recipe_ingredients (
        recipe_id bigint NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
        position integer NOT NULL CHECK (position > 0),
        name text NOT NULL,
        quantity numeric,
        unit text NOT NULL DEFAULT '',
        PRIMARY KEY (recipe_id, position),
        CONSTRAINT recipe_ingredients_name_check CHECK (
            char_length(name) BETWEEN 1 AND 200
            AND btrim(name, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
            AND name !~ U&'[\0001-\001F\007F-\009F]'
        ),
        CONSTRAINT recipe_ingredients_quantity_check CHECK (
            quantity IS NULL OR (
                quantity BETWEEN 0.001 AND 999999999.999
                AND quantity = trunc(quantity, 3)
            )
        ),
        CONSTRAINT recipe_ingredients_unit_check CHECK (
            char_length(unit) <= 32 AND unit !~ U&'[\0001-\001F\007F-\009F]'
        )
    );

    CREATE TABLE recipe_steps (
        recipe_id bigint NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
        position integer NOT NULL CHECK (position > 0),
        text text NOT NULL,
        PRIMARY KEY (recipe_id, position),
        CONSTRAINT recipe_steps_text_check CHECK (
            char_length(text) BETWEEN 1 AND 20000
            AND btrim(text, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
            AND text !~ U&'[\0001-\0008\000B-\001F\007F-\009F]'
        )
    );

    CREATE TABLE recipe_idempotency (
        owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        key text NOT NULL CHECK (key ~ '^[A-Za-z0-9_-]{16,128}$'),
        request_hash text NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
        resource_id bigint,
        response jsonb,
        expires_at timestamptz NOT NULL,
        PRIMARY KEY (owner_id, key),
        CHECK ((resource_id IS NULL) = (response IS NULL)),
        CHECK (resource_id IS NULL OR resource_id > 0),
        CHECK (response IS NULL OR jsonb_typeof(response) = 'object')
    );
    CREATE INDEX recipe_idempotency_expires_at_idx ON recipe_idempotency (expires_at);
END
$migration$;
