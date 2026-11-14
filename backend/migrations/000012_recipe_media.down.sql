DO $migration$
BEGIN
    RAISE EXCEPTION 'recipe media rollback requires a verified backup of database and objects' USING ERRCODE = '0A000';
END
$migration$;
