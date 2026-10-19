DO $$
BEGIN
    RAISE EXCEPTION 'Email activation hashes cannot be converted back to plaintext tokens';
END $$;
