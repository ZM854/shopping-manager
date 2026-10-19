DO $$ BEGIN
    RAISE EXCEPTION 'Downgrade of refresh sessions is unsupported; restore a backup if required';
END $$;
