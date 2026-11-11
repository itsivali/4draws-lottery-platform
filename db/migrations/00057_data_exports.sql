-- +goose Up
-- A player's copy of their data, sent to their email address (ADR 0038). Each sending is recorded: when, how large
-- the file was and its SHA-256, never its contents. Append-only; the latest row per player limits how often they ask.
CREATE TABLE identity.data_exports (
  id            uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  user_id       uuid NOT NULL REFERENCES identity.users (id),
  requested_at  timestamptz NOT NULL,
  bytes         integer NOT NULL CHECK (bytes > 0),
  sha256        text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$')
);
CREATE INDEX data_exports_user_idx ON identity.data_exports (user_id, requested_at DESC);
CREATE TRIGGER data_exports_append_only BEFORE UPDATE OR DELETE ON identity.data_exports
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON identity.data_exports TO app_api;
  GRANT SELECT ON identity.data_exports TO app_worker, app_readonly;
  ALTER TABLE identity.data_exports OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE identity.data_exports;
