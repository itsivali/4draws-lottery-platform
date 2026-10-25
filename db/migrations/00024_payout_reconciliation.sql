-- +goose Up
-- Phase 5 slice 3: prize payouts, the KRA withholding tax feed and the daily payout reconciliation.

-- One withholding tax event per paid payout, however often settlement runs.
CREATE UNIQUE INDEX kra_outbox_withholding_once ON audit.kra_outbox ((payload->>'payout_id')) WHERE event_type = 'withholding_tax';

-- Payout reconciliation runs (payouts ⇔ journals ⇔ B2C_FLOAT ⇔ TAX_PAYABLE ⇔ the KRA feed), append-only like the others.
CREATE TABLE payouts.reconciliation_runs (
  id      uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  ran_at  timestamptz NOT NULL DEFAULT now(),
  ok      boolean NOT NULL,
  report  jsonb NOT NULL
);
CREATE TRIGGER payouts_reconciliation_runs_append_only BEFORE UPDATE OR DELETE ON payouts.reconciliation_runs
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON payouts.reconciliation_runs TO app_api, app_worker;
  GRANT SELECT ON payouts.reconciliation_runs TO app_readonly;
  ALTER TABLE payouts.reconciliation_runs OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE payouts.reconciliation_runs;
DROP INDEX audit.kra_outbox_withholding_once;
