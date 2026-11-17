BEGIN;

CREATE TABLE IF NOT EXISTS system_runtime_config (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    registration_enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMIT;
