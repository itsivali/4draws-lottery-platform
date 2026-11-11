-- +goose Up
-- ADR 0036 (owner, 2026-11-11): switches on the System page that pause one money area (top-ups, plays, withdrawals,
-- payouts) without stopping the rest. A switch only ever restricts: it cannot move money the operating mode
-- (ADR 0027) does not allow. Append-only; an area's latest row is its state, and with none it runs.
CREATE TABLE compliance.area_switches (
  id          uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  area        text NOT NULL CHECK (area IN ('top-ups','plays','withdrawals','payouts')),
  paused      boolean NOT NULL,
  reason      text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
  changed_by  text NOT NULL CHECK (changed_by LIKE 'staff:%'),
  created_at  timestamptz NOT NULL
);
CREATE INDEX area_switches_latest_idx ON compliance.area_switches (area, created_at DESC, id DESC);
CREATE TRIGGER area_switches_append_only BEFORE UPDATE OR DELETE ON compliance.area_switches
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON compliance.area_switches TO app_api, app_worker;
  GRANT SELECT ON compliance.area_switches TO app_readonly;
  ALTER TABLE compliance.area_switches OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE compliance.area_switches;
