-- Additive upgrade for existing production databases; run with psql ON_ERROR_STOP.
-- Fresh databases receive the same structures from 000001_initial_schema.
BEGIN;
SET LOCAL lock_timeout = '10s';
ALTER TABLE local_account ADD COLUMN IF NOT EXISTS default_language text NOT NULL
    DEFAULT 'zh-Hans' CHECK (default_language IN ('zh-Hans','en'));
CREATE TABLE IF NOT EXISTS page_translation (
    source_page_id uuid PRIMARY KEY REFERENCES page(id),
    source_revision_id uuid NOT NULL REFERENCES revision(id),
    target_language text NOT NULL CHECK (target_language IN ('zh-Hans','en')),
    target_page_id uuid UNIQUE REFERENCES page(id),
    requested_by uuid NOT NULL REFERENCES actor(id),
    event_id uuid NOT NULL REFERENCES outbox_event(id),
    status text NOT NULL CHECK (status IN ('pending','failed','done')),
    CHECK (source_page_id <> target_page_id)
);
COMMIT;
