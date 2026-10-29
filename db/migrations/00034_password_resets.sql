-- +goose Up
-- Owner, 2026-10-29: staff reset a forgotten back-office password themselves through an emailed link, and staff can give
-- a player a temporary password from the back office (players have no email), which the player must change next.

-- One row per emailed link: only the SHA-256 of the token is kept, a link lives 30 minutes and is used at most once.
CREATE TABLE staff.password_resets (
  id          uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  member_id   uuid NOT NULL REFERENCES staff.members (id),
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  created_at  timestamptz NOT NULL,
  CONSTRAINT password_resets_expiry_after_creation CHECK (expires_at > created_at)
);
CREATE INDEX password_resets_member_idx ON staff.password_resets (member_id) WHERE used_at IS NULL;

-- A player given a temporary password by staff must choose their own at the next sign-in (the app insists).
ALTER TABLE identity.users ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT, UPDATE ON staff.password_resets TO app_worker;
  ALTER TABLE staff.password_resets OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE identity.users DROP COLUMN must_change_password;
DROP TABLE staff.password_resets;
