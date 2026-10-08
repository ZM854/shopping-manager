-- Один DO-блок делает проверку и перенос атомарными даже без внешней транзакции.
-- Развёртывать только вместе с backend, использующим shopping_items (этап 4).
DO $migration$
DECLARE
    invalid_count bigint;
    -- Тот же набор пробелов, что у Go strings.TrimSpace. Исходный текст не обрезаем.
    whitespace text := E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000';
BEGIN
    -- Запрещаем изменения исходных данных до окончания проверки и переноса.
    LOCK TABLE users, products IN SHARE ROW EXCLUSIVE MODE;
    -- Представление float8 не должно зависеть от настройки сессии и терять цифры.
    PERFORM set_config('extra_float_digits', '3', true);

    SELECT count(*) INTO invalid_count FROM products
    WHERE char_length(name) NOT BETWEEN 1 AND 200
       OR btrim(name, whitespace) = ''
       OR name ~ U&'[\0001-\001F\007F-\009F]';
    IF invalid_count > 0 THEN
        RAISE EXCEPTION 'migration 000004: invalid product names (% rows)', invalid_count
            USING ERRCODE = '23514', HINT = 'Review legacy data explicitly; migration does not trim or delete rows.';
    END IF;

    SELECT count(*) INTO invalid_count FROM products
    WHERE char_length(unit) > 32 OR unit ~ U&'[\0001-\001F\007F-\009F]';
    IF invalid_count > 0 THEN
        RAISE EXCEPTION 'migration 000004: invalid product units (% rows)', invalid_count
            USING ERRCODE = '23514';
    END IF;

    SELECT count(*) INTO invalid_count FROM products
    WHERE CASE WHEN quantity::text IN ('NaN', 'Infinity', '-Infinity') THEN true
               ELSE quantity::text::numeric NOT BETWEEN 0.001 AND 999999999.999
                 OR quantity::text::numeric <> trunc(quantity::text::numeric, 3)
          END;
    IF invalid_count > 0 THEN
        RAISE EXCEPTION 'migration 000004: invalid product quantities (% rows)', invalid_count
            USING ERRCODE = '23514', HINT = 'Expected finite decimal 0.001..999999999.999 with at most 3 fractional digits; no rounding is performed.';
    END IF;

    CREATE TABLE shopping_lists (
        id bigserial PRIMARY KEY,
        owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        name text NOT NULL,
        created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
        CONSTRAINT shopping_lists_name_check CHECK (
            char_length(name) BETWEEN 1 AND 100
            AND btrim(name, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
            AND name !~ U&'[\0001-\001F\007F-\009F]'
        )
    );
    CREATE INDEX shopping_lists_owner_id_id_idx ON shopping_lists (owner_id, id);

    INSERT INTO shopping_lists (owner_id, name)
    SELECT id, 'Покупки' FROM users ORDER BY id;

    ALTER TABLE products RENAME TO shopping_items;
    ALTER TABLE shopping_items RENAME CONSTRAINT products_pkey TO shopping_items_pkey;
    ALTER SEQUENCE products_id_seq RENAME TO shopping_items_id_seq;
    ALTER TABLE shopping_items ADD COLUMN list_id bigint;
    UPDATE shopping_items AS item SET list_id = list.id
    FROM shopping_lists AS list WHERE list.owner_id = item.user_id;
    ALTER TABLE shopping_items
        ALTER COLUMN list_id SET NOT NULL,
        ADD CONSTRAINT shopping_items_list_id_fkey FOREIGN KEY (list_id)
            REFERENCES shopping_lists(id) ON DELETE CASCADE,
        DROP COLUMN user_id,
        -- text сохраняет десятичное представление float8 без округления NUMERIC(p,s).
        ALTER COLUMN quantity TYPE numeric USING quantity::text::numeric,
        ALTER COLUMN quantity DROP NOT NULL,
        ALTER COLUMN unit SET DEFAULT '',
        ADD CONSTRAINT shopping_items_name_check CHECK (
            char_length(name) BETWEEN 1 AND 200
            AND btrim(name, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
            AND name !~ U&'[\0001-\001F\007F-\009F]'
        ),
        ADD CONSTRAINT shopping_items_unit_check CHECK (
            char_length(unit) <= 32 AND unit !~ U&'[\0001-\001F\007F-\009F]'
        ),
        ADD CONSTRAINT shopping_items_quantity_check CHECK (
            quantity IS NULL OR (quantity BETWEEN 0.001 AND 999999999.999
                                 AND quantity = trunc(quantity, 3))
        );
    CREATE INDEX shopping_items_list_id_id_idx ON shopping_items (list_id, id);
END
$migration$;
