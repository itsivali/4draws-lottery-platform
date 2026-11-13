-- +goose Up
-- Telling a winner by email (ADR 0042). One row per prize says what was done about telling its winner: emailed, or
-- skipped with the reason (no confirmed address, a closed account, a prize from before this existed). A prize has
-- at most one row, so nobody is told twice. Append-only.
CREATE TABLE draws.prize_notices (
  result_id   uuid PRIMARY KEY REFERENCES draws.draw_results (id),
  outcome     text NOT NULL CHECK (outcome IN ('emailed','skipped')),
  reason      text CHECK ((outcome = 'skipped') = (reason IS NOT NULL)),
  created_at  timestamptz NOT NULL
);
CREATE TRIGGER prize_notices_append_only BEFORE UPDATE OR DELETE ON draws.prize_notices
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- prizes won before this existed are not announced now
INSERT INTO draws.prize_notices (result_id, outcome, reason, created_at)
SELECT r.id, 'skipped', 'won before notices existed', now() FROM draws.draw_results r;

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON draws.prize_notices TO app_worker;
  GRANT SELECT ON draws.prize_notices TO app_api, app_readonly;
  ALTER TABLE draws.prize_notices OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE draws.prize_notices;
