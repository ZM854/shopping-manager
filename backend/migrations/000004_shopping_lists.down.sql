DO $migration$
BEGIN
    RAISE EXCEPTION 'migration 000004 cannot be reversed without data loss';
END
$migration$;
