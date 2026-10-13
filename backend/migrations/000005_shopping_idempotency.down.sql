DO $migration$
BEGIN
    RAISE EXCEPTION 'migration 000005 cannot be reversed without losing POST replay guarantees; restore a verified backup with the matching application version';
END
$migration$;
