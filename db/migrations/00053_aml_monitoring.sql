-- +goose Up
-- ADR 0033: transaction monitoring. Rules (thresholds pending the compliance owner's approval) raise cases about
-- players; staff look into them, may hold the account's money (no top-ups, paybill payments, withdrawals or payouts)
-- while they do, and close them with an outcome. A foundation for the compliance programme, not a claim that it meets
-- POCAMLA: the compliance owner decides the rules, the thresholds and what is reported to the FRC.

-- Each change of a rule is a new row; the latest row of a rule is in force (the code's defaults until one exists).
CREATE TABLE compliance.aml_rule_settings (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  rule               text NOT NULL CHECK (rule IN ('deposit_velocity','in_out_little_play','failed_topups','shared_id_number','shared_device')),
  enabled            boolean NOT NULL,
  params             jsonb NOT NULL,
  approval_reference text,  -- the compliance owner's approval of these thresholds; NULL: pending
  set_by             text NOT NULL CHECK (set_by ~ '^staff:'),
  note               text NOT NULL CHECK (length(note) BETWEEN 1 AND 1000),
  set_at             timestamptz NOT NULL
);
CREATE INDEX aml_rule_settings_rule_idx ON compliance.aml_rule_settings (rule, id);
CREATE TRIGGER aml_rule_settings_append_only BEFORE UPDATE OR DELETE ON compliance.aml_rule_settings
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

CREATE TABLE compliance.aml_cases (
  id         uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  user_id    uuid NOT NULL REFERENCES identity.users (id),
  rule       text NOT NULL CHECK (rule IN ('deposit_velocity','in_out_little_play','failed_topups','shared_id_number','shared_device','manual')),
  status     text NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
  hold       boolean NOT NULL DEFAULT false,
  hold_by    text,
  evidence   jsonb NOT NULL,
  opened_by  text,          -- the staff member who opened a manual case; NULL for a rule
  outcome    text,          -- the closing note
  opened_at  timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  closed_at  timestamptz,
  closed_by  text,
  CONSTRAINT aml_cases_hold_is_open CHECK (NOT hold OR status = 'open'),
  CONSTRAINT aml_cases_hold_named CHECK (NOT hold OR hold_by IS NOT NULL),
  CONSTRAINT aml_cases_closed CHECK ((status = 'closed') = (closed_at IS NOT NULL AND closed_by IS NOT NULL AND outcome IS NOT NULL))
);
-- one open case per player and rule
CREATE UNIQUE INDEX aml_cases_open_once ON compliance.aml_cases (user_id, rule) WHERE status = 'open';
CREATE INDEX aml_cases_status_idx ON compliance.aml_cases (status, opened_at, id);
CREATE INDEX aml_cases_hold_idx ON compliance.aml_cases (user_id) WHERE hold;

-- what a case is about never changes; a closed case is final
-- +goose StatementBegin
CREATE FUNCTION compliance.check_aml_case() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.user_id, NEW.rule, NEW.evidence, NEW.opened_by, NEW.opened_at)
     IS DISTINCT FROM (OLD.id, OLD.user_id, OLD.rule, OLD.evidence, OLD.opened_by, OLD.opened_at) THEN
    RAISE EXCEPTION 'aml case %: what it is about cannot change', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF OLD.status = 'closed' THEN
    RAISE EXCEPTION 'aml case %: a closed case is final', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER aml_cases_check BEFORE UPDATE ON compliance.aml_cases
  FOR EACH ROW EXECUTE FUNCTION compliance.check_aml_case();
CREATE TRIGGER aml_cases_no_delete BEFORE DELETE ON compliance.aml_cases
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- everything done on a case, in order
CREATE TABLE compliance.aml_case_events (
  id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id  uuid NOT NULL REFERENCES compliance.aml_cases (id),
  action   text NOT NULL CHECK (action IN ('opened','hold','released','note','closed')),
  staff    text,
  note     text,
  at       timestamptz NOT NULL
);
CREATE INDEX aml_case_events_case_idx ON compliance.aml_case_events (case_id, id);
CREATE TRIGGER aml_case_events_append_only BEFORE UPDATE OR DELETE ON compliance.aml_case_events
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON compliance.aml_rule_settings, compliance.aml_case_events TO app_api, app_worker;
  GRANT SELECT, INSERT, UPDATE ON compliance.aml_cases TO app_api, app_worker;
  GRANT SELECT ON compliance.aml_rule_settings, compliance.aml_cases, compliance.aml_case_events TO app_readonly;
  ALTER TABLE compliance.aml_rule_settings OWNER TO app_migrator;
  ALTER TABLE compliance.aml_cases OWNER TO app_migrator;
  ALTER TABLE compliance.aml_case_events OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE compliance.aml_case_events;
DROP TABLE compliance.aml_cases;
DROP FUNCTION compliance.check_aml_case();
DROP TABLE compliance.aml_rule_settings;
