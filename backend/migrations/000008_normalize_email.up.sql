-- Unicode White_Space соответствует strings.TrimSpace в Go.
DO $$
DECLARE
    whitespace text := chr(9)||chr(10)||chr(11)||chr(12)||chr(13)||chr(32)||
        chr(133)||chr(160)||chr(5760)||chr(8192)||chr(8193)||chr(8194)||
        chr(8195)||chr(8196)||chr(8197)||chr(8198)||chr(8199)||chr(8200)||
        chr(8201)||chr(8202)||chr(8232)||chr(8233)||chr(8239)||chr(8287)||chr(12288);
BEGIN
    LOCK TABLE users IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (
        SELECT 1 FROM users GROUP BY lower(btrim(email, whitespace)) HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'email normalization conflict: resolve duplicate accounts before migration';
    END IF;
    IF EXISTS (
        SELECT 1 FROM users WHERE octet_length(lower(btrim(email, whitespace))) NOT BETWEEN 1 AND 254
    ) THEN
        RAISE EXCEPTION 'email normalization failed: address length outside 1..254 bytes';
    END IF;
    UPDATE users SET email = lower(btrim(email, whitespace));
    ALTER TABLE users ADD CONSTRAINT users_email_normalized CHECK (
        email = lower(btrim(email, chr(9)||chr(10)||chr(11)||chr(12)||chr(13)||chr(32)||
        chr(133)||chr(160)||chr(5760)||chr(8192)||chr(8193)||chr(8194)||
        chr(8195)||chr(8196)||chr(8197)||chr(8198)||chr(8199)||chr(8200)||
        chr(8201)||chr(8202)||chr(8232)||chr(8233)||chr(8239)||chr(8287)||chr(12288)))
    );
END $$;

