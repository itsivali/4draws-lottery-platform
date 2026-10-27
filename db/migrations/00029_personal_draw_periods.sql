-- +goose Up
-- Owner, 2026-10-26 (ADR 0017): weekly, monthly and quarterly draws follow each player's own periods, counted from
-- their first play (7, 30 and 90 days, back to back). Daily draws are unchanged.

-- A player's first play day, recorded once by the play itself; periods are counted from it and it never moves.
CREATE TABLE tickets.player_starts (
  user_id          uuid PRIMARY KEY REFERENCES identity.users (id),
  first_play_date  date NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX player_starts_first_play_idx ON tickets.player_starts (first_play_date);
CREATE TRIGGER player_starts_append_only BEFORE UPDATE OR DELETE ON tickets.player_starts
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();
CREATE TRIGGER player_starts_no_truncate BEFORE TRUNCATE ON tickets.player_starts
  FOR EACH STATEMENT EXECUTE FUNCTION platform.forbid_mutation();

INSERT INTO tickets.player_starts (user_id, first_play_date)
SELECT user_id, min(play_date) FROM tickets.player_days WHERE paid_plays > 0 GROUP BY user_id;

-- calendar: the period is everyone's; personal: the players whose own period is this window (ends on period_end)
ALTER TABLE draws.draws
  ADD COLUMN basis text NOT NULL DEFAULT 'calendar' CHECK (basis IN ('calendar','personal')),
  ADD CONSTRAINT draws_daily_is_calendar CHECK (basis = 'calendar' OR draw_type <> 'daily');
DROP INDEX draws.draws_one_live_per_period;
CREATE UNIQUE INDEX draws_one_live_per_period ON draws.draws (draw_type, basis, period_start) WHERE status <> 'voided';

-- Open calendar draws of the longer types would count play days the personal draws now count: void them. Frozen ones
-- finish as they are; their play days are not entered again.
UPDATE draws.draws SET status = 'voided', voided_reason = 'draw periods changed to each player''s own (ADR 0017)', updated_at = now()
WHERE status = 'open' AND basis = 'calendar' AND draw_type <> 'daily';

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  -- the first play records the start (the API); the draw scheduler reads it (the worker)
  GRANT SELECT, INSERT ON tickets.player_starts TO app_api, app_worker;
  GRANT SELECT ON tickets.player_starts TO app_readonly;
  ALTER TABLE tickets.player_starts OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP INDEX draws.draws_one_live_per_period;
CREATE UNIQUE INDEX draws_one_live_per_period ON draws.draws (draw_type, period_start) WHERE status <> 'voided';
ALTER TABLE draws.draws DROP CONSTRAINT draws_daily_is_calendar, DROP COLUMN basis;
DROP TABLE tickets.player_starts;
