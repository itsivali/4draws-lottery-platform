-- +goose Up
-- Owner, 2026-10-26 (ADR 0018): staff sign in with Google or Apple; who they are and what they may do is kept here,
-- managed by admins in the back office, instead of in Keycloak. A member is invited by email; their first sign-in
-- with a verified address binds that provider identity to them.
CREATE SCHEMA IF NOT EXISTS staff;

CREATE TABLE staff.members (
  id          uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  email       text NOT NULL CHECK (email = lower(email) AND email ~ '^[^@[:space:]]+@[^@[:space:]]+$'),
  name        text NOT NULL DEFAULT '',
  roles       text[] NOT NULL DEFAULT '{}' CHECK (roles <@ ARRAY['admin','finance','kyc_reviewer','payments_operator',
                'draw_official','payout_operator','tax_officer','alerts_operator','support','auditor']::text[]),
  active      boolean NOT NULL DEFAULT true,
  invited_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT members_email_key UNIQUE (email)
);
CREATE TRIGGER members_no_delete BEFORE DELETE ON staff.members
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- A provider identity (Google or Apple subject) bound to a member at their first sign-in; never rebound.
CREATE TABLE staff.identities (
  provider    text NOT NULL,
  subject     text NOT NULL,
  member_id   uuid NOT NULL REFERENCES staff.members (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (provider, subject)
);
CREATE TRIGGER identities_append_only BEFORE UPDATE OR DELETE ON staff.identities
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- Back-office sessions: only the SHA-256 of the token is stored.
CREATE TABLE staff.sessions (
  id            uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  token_hash    bytea NOT NULL UNIQUE,
  member_id     uuid NOT NULL REFERENCES staff.members (id),
  provider      text NOT NULL,
  created_at    timestamptz NOT NULL,
  expires_at    timestamptz NOT NULL,
  last_seen_at  timestamptz NOT NULL,
  revoked_at    timestamptz
);
CREATE INDEX sessions_member_idx ON staff.sessions (member_id) WHERE revoked_at IS NULL;

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  -- the back office runs with the worker's role (ADR 0013)
  GRANT USAGE ON SCHEMA staff TO app_worker, app_readonly;
  GRANT SELECT, INSERT, UPDATE ON staff.members, staff.sessions TO app_worker;
  GRANT SELECT, INSERT ON staff.identities TO app_worker;
  GRANT SELECT ON staff.members, staff.identities TO app_readonly;
  ALTER SCHEMA staff OWNER TO app_migrator; -- foreign-key checks run as the tables' owner
  ALTER TABLE staff.members OWNER TO app_migrator;
  ALTER TABLE staff.identities OWNER TO app_migrator;
  ALTER TABLE staff.sessions OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE staff.sessions, staff.identities, staff.members;
DROP SCHEMA staff;
