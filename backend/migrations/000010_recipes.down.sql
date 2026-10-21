-- Удаление рецептов и результатов POST необратимо без восстановления данных.
DO $$ BEGIN
    RAISE EXCEPTION '000010 is irreversible; restore a verified backup if rollback is required'
        USING ERRCODE = '0A000';
END $$;
