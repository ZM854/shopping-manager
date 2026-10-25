DO $$ BEGIN
    RAISE EXCEPTION '000011 is irreversible; restore a verified backup if rollback is required'
        USING ERRCODE = '0A000';
END $$;
