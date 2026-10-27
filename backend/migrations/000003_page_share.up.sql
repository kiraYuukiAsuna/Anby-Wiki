BEGIN;

CREATE TABLE IF NOT EXISTS page_share (
    page_id    uuid        NOT NULL REFERENCES page (id),
    actor_id   uuid        NOT NULL REFERENCES actor (id),
    role_key   text        NOT NULL CHECK (role_key IN ('viewer', 'editor')),
    granted_by uuid        NOT NULL REFERENCES actor (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, actor_id)
);

CREATE INDEX IF NOT EXISTS page_share_actor_page_idx ON page_share (actor_id, page_id);

COMMIT;
