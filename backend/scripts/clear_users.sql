DELETE FROM refresh_tokens;
DELETE FROM products;
DELETE FROM users;

ALTER SEQUENCE users_id_seq RESTART WITH 1;
ALTER SEQUENCE refresh_tokens_id_seq RESTART WITH 1;
ALTER SEQUENCE products_id_seq RESTART WITH 1;