DO $migration$
BEGIN
    ALTER TABLE recipes ADD CONSTRAINT recipes_owner_id_id_key UNIQUE (owner_id, id);

    CREATE TABLE recipe_media (
        id bigserial PRIMARY KEY CHECK (id > 0),
        owner_id bigint REFERENCES users(id) ON DELETE SET NULL,
        purpose text NOT NULL CHECK (purpose IN ('cover', 'step-image', 'step-video')),
        byte_size bigint NOT NULL,
        sha256 text NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
        state text NOT NULL DEFAULT 'awaiting_upload'
            CHECK (state IN ('awaiting_upload', 'processing', 'ready', 'attached', 'failed', 'delete_pending')),
        created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
        updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
        expires_at timestamptz,
        error_code text,
        accounted_bytes bigint NOT NULL,
        source_reserved_bytes bigint NOT NULL,
        upload_slot_reserved boolean NOT NULL DEFAULT true,
        recipe_id bigint,
        step_id bigint,
        position integer,
        UNIQUE (id, purpose),
        FOREIGN KEY (owner_id, recipe_id) REFERENCES recipes(owner_id, id),
        FOREIGN KEY (recipe_id, step_id) REFERENCES recipe_steps(recipe_id, id),
        CHECK (owner_id IS NOT NULL OR state = 'delete_pending'),
        CHECK (byte_size BETWEEN 1 AND CASE WHEN purpose = 'step-video' THEN 52428800 ELSE 15728640 END),
        CHECK (accounted_bytes BETWEEN 1 AND CASE purpose
            WHEN 'cover' THEN 204800 WHEN 'step-image' THEN 1536000 ELSE 21176320 END),
        CHECK (source_reserved_bytes IN (0, byte_size)),
        CHECK (expires_at IS NULL OR expires_at > created_at),
        CHECK ((state = 'attached') = (expires_at IS NULL)),
        CHECK (state <> 'attached' OR (expires_at IS NULL AND NOT upload_slot_reserved AND source_reserved_bytes = 0)),
        CHECK ((state = 'failed' AND error_code IN (
            'invalid_media', 'unsupported_media_type', 'media_constraints_exceeded',
            'media_output_too_large', 'media_storage_unavailable', 'media_processor_unavailable',
            'media_processing_timeout', 'media_processing_interrupted'
        ) AND error_code IS NOT NULL) OR (state <> 'failed' AND error_code IS NULL)),
        CHECK (
            (state <> 'attached' AND recipe_id IS NULL AND step_id IS NULL AND position IS NULL)
            OR (state = 'attached' AND recipe_id IS NOT NULL AND owner_id IS NOT NULL AND (
                (purpose = 'cover' AND step_id IS NULL AND position IS NULL)
                OR (purpose <> 'cover' AND step_id IS NOT NULL AND position BETWEEN 1 AND 4 AND position IS NOT NULL)
            ))
        )
    );
    CREATE INDEX recipe_media_owner_id_id_idx ON recipe_media (owner_id, id);
    CREATE INDEX recipe_media_cleanup_idx ON recipe_media (state, expires_at, id);
    CREATE INDEX recipe_media_recipe_id_idx ON recipe_media (recipe_id) WHERE recipe_id IS NOT NULL;
    CREATE UNIQUE INDEX recipe_media_cover_idx ON recipe_media (recipe_id) WHERE state = 'attached' AND purpose = 'cover';
    CREATE UNIQUE INDEX recipe_media_step_position_idx ON recipe_media (step_id, position) WHERE state = 'attached';
    CREATE UNIQUE INDEX recipe_media_step_video_idx ON recipe_media (step_id) WHERE state = 'attached' AND purpose = 'step-video';

    -- Planned keys exist before the first S3 write, including unsuccessful uploads.
    CREATE TABLE recipe_media_variants (
        media_id bigint NOT NULL,
        purpose text NOT NULL,
        name text NOT NULL,
        object_key text NOT NULL UNIQUE,
        content_type text,
        byte_size bigint,
        width integer,
        height integer,
        duration_milliseconds bigint,
        PRIMARY KEY (media_id, name),
        FOREIGN KEY (media_id, purpose) REFERENCES recipe_media(id, purpose) ON DELETE CASCADE,
        CHECK ((purpose = 'cover' AND name = 'cover') OR (purpose = 'step-image' AND name = 'image')
            OR (purpose = 'step-video' AND name IN ('video', 'poster'))),
        CHECK (object_key = 'recipe-media/' || media_id::text || '/' || name || CASE WHEN name = 'video' THEN '.mp4' ELSE '.jpg' END),
        CHECK (
            (content_type IS NULL AND byte_size IS NULL AND width IS NULL AND height IS NULL AND duration_milliseconds IS NULL)
            OR (content_type IS NOT NULL AND byte_size IS NOT NULL AND width IS NOT NULL AND height IS NOT NULL
                AND byte_size > 0 AND width > 0 AND height > 0 AND (
                    (name = 'cover' AND content_type = 'image/jpeg' AND byte_size <= 204800 AND width = 640 AND height = 360 AND duration_milliseconds IS NULL)
                    OR (name = 'image' AND content_type = 'image/jpeg' AND byte_size <= 1536000 AND width <= 1600 AND height <= 1600 AND duration_milliseconds IS NULL)
                    OR (name = 'poster' AND content_type = 'image/jpeg' AND byte_size <= 204800
                        AND ((width <= 640 AND height <= 360) OR (width <= 360 AND height <= 640)) AND duration_milliseconds IS NULL)
                    OR (name = 'video' AND content_type = 'video/mp4' AND byte_size <= 20971520
                        AND ((width <= 1280 AND height <= 720) OR (width <= 720 AND height <= 1280))
                        AND width % 2 = 0 AND height % 2 = 0 AND duration_milliseconds IS NOT NULL AND duration_milliseconds BETWEEN 1 AND 60000)
                )
            )
        )
    );
    CREATE TABLE recipe_media_idempotency (
        owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        key text NOT NULL CHECK (key ~ '^[A-Za-z0-9_-]{16,128}$'),
        request_hash text NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
        resource_id bigint,
        response jsonb,
        expires_at timestamptz NOT NULL,
        PRIMARY KEY (owner_id, key),
        CHECK ((resource_id IS NULL) = (response IS NULL)),
        CHECK (resource_id IS NULL OR resource_id > 0),
        CHECK (response IS NULL OR jsonb_typeof(response) = 'object')
    );
    CREATE INDEX recipe_media_idempotency_expiry_idx ON recipe_media_idempotency (expires_at);

    -- Parent deletion must preserve keys and quota until physical removal.
    CREATE FUNCTION recipe_media_parent_deleted() RETURNS trigger LANGUAGE plpgsql AS $body$
    BEGIN
        IF TG_TABLE_NAME = 'users' THEN
            UPDATE recipe_media SET state = 'delete_pending', error_code = NULL,
                recipe_id = NULL, step_id = NULL, position = NULL,
                expires_at = COALESCE(expires_at, created_at + interval '24 hours'), updated_at = clock_timestamp()
                WHERE owner_id = OLD.id;
        ELSIF TG_TABLE_NAME = 'recipes' THEN
            UPDATE recipe_media SET state = 'delete_pending', error_code = NULL,
                recipe_id = NULL, step_id = NULL, position = NULL,
                expires_at = COALESCE(expires_at, created_at + interval '24 hours'), updated_at = clock_timestamp()
                WHERE recipe_id = OLD.id;
        ELSE
            UPDATE recipe_media SET state = 'delete_pending', error_code = NULL,
                recipe_id = NULL, step_id = NULL, position = NULL,
                expires_at = COALESCE(expires_at, created_at + interval '24 hours'), updated_at = clock_timestamp()
                WHERE recipe_id = OLD.recipe_id AND step_id = OLD.id;
        END IF;
        RETURN OLD;
    END
    $body$;
    CREATE TRIGGER recipe_media_user_deleted BEFORE DELETE ON users
        FOR EACH ROW EXECUTE FUNCTION recipe_media_parent_deleted();
    CREATE TRIGGER recipe_media_recipe_deleted BEFORE DELETE ON recipes
        FOR EACH ROW EXECUTE FUNCTION recipe_media_parent_deleted();
    CREATE TRIGGER recipe_media_step_deleted BEFORE DELETE ON recipe_steps
        FOR EACH ROW EXECUTE FUNCTION recipe_media_parent_deleted();
END
$migration$;
