BEGIN;

CREATE TABLE IF NOT EXISTS page_share_link (
    id          uuid        PRIMARY KEY,
    page_id     uuid        NOT NULL REFERENCES page (id),
    token_hash  bytea       NOT NULL UNIQUE,
    role_key    text        NOT NULL CHECK (role_key IN ('viewer', 'editor')),
    link_type   text        NOT NULL CHECK (link_type IN ('once', 'permanent')),
    created_by  uuid        NOT NULL REFERENCES actor (id),
    claimed_by  uuid        REFERENCES actor (id),
    claimed_at  timestamptz,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (link_type = 'once') OR
        (claimed_by IS NULL AND claimed_at IS NULL)
    ),
    CHECK (
        (claimed_by IS NULL AND claimed_at IS NULL) OR
        (claimed_by IS NOT NULL AND claimed_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS page_share_link_page_created_idx ON page_share_link (page_id, created_at DESC);

COMMIT;
