ALTER TABLE products
ADD COLUMN user_id bigint NOT NULL REFERENCES users(id);