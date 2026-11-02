-- Explicit additive upgrade for an existing version-4 deployment.
-- Application startup never executes this file. Run before deploying the API.
-- Fresh databases receive the same objects from 000001_initial_schema.up.sql.
BEGIN;

ALTER TABLE local_account
    ADD COLUMN IF NOT EXISTS email_verified_at timestamptz,
    ADD COLUMN IF NOT EXISTS system_admin boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS totp_ciphertext text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS totp_pending_ciphertext text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS totp_pending_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS totp_last_step bigint NOT NULL DEFAULT -1,
    ADD COLUMN IF NOT EXISTS security_failures integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS security_locked_until timestamptz;

ALTER TABLE auth_session
    ADD COLUMN IF NOT EXISTS user_agent text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_seen_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS mfa_verified boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS account_security_token (
    token_hash bytea PRIMARY KEY,
    actor_id uuid NOT NULL REFERENCES actor (id) ON DELETE CASCADE,
    purpose text NOT NULL CHECK (purpose IN ('verify_email','change_email','reset_password','mfa_login')),
    target text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_security_token_actor_idx
    ON account_security_token (actor_id, purpose, created_at);

CREATE TABLE IF NOT EXISTS account_recovery_code (
    actor_id uuid NOT NULL REFERENCES actor (id) ON DELETE CASCADE,
    code_hash bytea NOT NULL,
    PRIMARY KEY (actor_id, code_hash)
);

CREATE TABLE IF NOT EXISTS system_mail_config (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    configuration jsonb NOT NULL,
    password_ciphertext text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS account_security_event (
    id uuid PRIMARY KEY,
    actor_id uuid REFERENCES actor (id) ON DELETE SET NULL,
    target_actor_id uuid REFERENCES actor (id) ON DELETE SET NULL,
    action text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_security_event_created_idx ON account_security_event (created_at DESC);

COMMIT;
