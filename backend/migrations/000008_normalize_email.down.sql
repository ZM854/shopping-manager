-- Нормализация регистра необратима без резервной копии; данные не восстанавливать догадками.
DO $$ BEGIN
    RAISE EXCEPTION '000008 is irreversible; restore a verified backup if rollback is required';
END $$;
