BEGIN;

ALTER TABLE actor_role
    DROP CONSTRAINT IF EXISTS actor_role_actor_id_wiki_id_fkey;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'actor_role'::regclass
          AND conname = 'actor_role_actor_id_fkey'
    ) THEN
        ALTER TABLE actor_role
            ADD CONSTRAINT actor_role_actor_id_fkey
            FOREIGN KEY (actor_id) REFERENCES actor (id);
    END IF;
END $$;

COMMIT;
