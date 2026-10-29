-- +goose Up
-- Owner, 2026-10-29: players give an email address at sign-up and reset a forgotten password themselves through an
-- emailed link (ADR 0021). The address is kept lower-cased and is unique among accounts; accounts from before have none
-- until the player adds one from the account screen (staff can still give them a temporary password).
ALTER TABLE identity.users ADD COLUMN email text
  CONSTRAINT users_email_format CHECK (email = lower(email) AND email ~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$');
CREATE UNIQUE INDEX users_email_key ON identity.users (email) WHERE email IS NOT NULL;

-- One row per emailed link: only the SHA-256 of the token is kept, a link lives 30 minutes and is used at most once.
CREATE TABLE identity.password_resets (
  id          uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  user_id     uuid NOT NULL REFERENCES identity.users (id),
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  created_at  timestamptz NOT NULL,
  CONSTRAINT password_resets_expiry_after_creation CHECK (expires_at > created_at)
);
CREATE INDEX password_resets_user_idx ON identity.password_resets (user_id) WHERE used_at IS NULL;

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT, UPDATE ON identity.password_resets TO app_api;
  GRANT SELECT, UPDATE ON identity.password_resets TO app_worker;
  GRANT SELECT ON identity.password_resets TO app_readonly;
  ALTER TABLE identity.password_resets OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE identity.password_resets;
DROP INDEX identity.users_email_key;
ALTER TABLE identity.users DROP COLUMN email;
