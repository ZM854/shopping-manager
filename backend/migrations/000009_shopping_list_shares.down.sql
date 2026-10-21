-- Снимки и гарантии повторов нельзя восстановить после удаления схемы обмена.
DO $$ BEGIN
    RAISE EXCEPTION '000009 is irreversible; restore a verified backup if rollback is required'
        USING ERRCODE = '0A000';
END $$;
