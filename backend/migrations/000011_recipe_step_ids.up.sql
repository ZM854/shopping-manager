DO $migration$
BEGIN
    ALTER TABLE recipe_steps
        ADD COLUMN id bigserial CHECK (id > 0),
        DROP CONSTRAINT recipe_steps_pkey,
        ADD PRIMARY KEY (id),
        ADD CONSTRAINT recipe_steps_recipe_position_key
            UNIQUE (recipe_id, position) DEFERRABLE INITIALLY DEFERRED,
        ADD CONSTRAINT recipe_steps_recipe_id_key UNIQUE (recipe_id, id);
END
$migration$;
