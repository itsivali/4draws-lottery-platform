-- +goose Up
-- Phase 5 slice 2: payouts through M-Pesa B2C (docs/checkpoints/phase-5.md).

-- held (waits for an operator) → pending (the send job is queued) → sending (committed before the B2C request) → sent
-- (Daraja accepted) | timed_out (outcome unknown) → result_received (a result claims an outcome) → succeeded | failed
-- (confirmed) | manual (an operator decides from the M-Pesa statement). A payout stuck in sending may have been paid:
-- it is checked, never sent again; one still pending was never sent and may be queued again.
ALTER TABLE payouts.payouts DROP CONSTRAINT payouts_status_check;
UPDATE payouts.payouts SET status = 'pending' WHERE status NOT IN ('pending','sent','timed_out','succeeded','failed','manual');
ALTER TABLE payouts.payouts ALTER COLUMN status SET DEFAULT 'pending',
  ADD CONSTRAINT payouts_status_check
    CHECK (status IN ('held','pending','sending','sent','timed_out','result_received','succeeded','failed','manual')),
  ADD COLUMN tax_code              text,
  ADD COLUMN tax_rate              numeric(6,5),
  ADD COLUMN tax_rounding          text CHECK (tax_rounding IN ('half_up','down','up')),
  ADD COLUMN status_requests       integer NOT NULL DEFAULT 0 CHECK (status_requests >= 0),
  ADD COLUMN status_requested_at   timestamptz,
  ADD COLUMN status_originator     text,
  ADD COLUMN reason                text,
  ADD COLUMN approved_by           text,
  ADD COLUMN settled_by            text,
  ADD COLUMN settled_at            timestamptz,
  ADD CONSTRAINT payouts_b2c_needs_msisdn CHECK (route <> 'b2c' OR msisdn IS NOT NULL),
  ADD CONSTRAINT payouts_succeeded_has_receipt CHECK (status <> 'succeeded' OR mpesa_receipt IS NOT NULL),
  ADD CONSTRAINT payouts_final_is_settled CHECK (status NOT IN ('succeeded','failed') OR settled_at IS NOT NULL),
  ADD CONSTRAINT payouts_prize_has_result CHECK ((kind = 'prize') = (draw_result_id IS NOT NULL));
CREATE INDEX payouts_user_created_idx ON payouts.payouts (user_id, created_at);
CREATE INDEX payouts_open_idx ON payouts.payouts (status) WHERE status NOT IN ('succeeded','failed');
-- A prize is paid once; a failed attempt (money returned to the pool) may be retried as a new payout.
DROP INDEX payouts.payouts_draw_result_once;
CREATE UNIQUE INDEX payouts_draw_result_once ON payouts.payouts (draw_result_id) WHERE draw_result_id IS NOT NULL AND status <> 'failed';

-- Only forward moves; what a payout pays, to whom, and its final state never change.
-- +goose StatementBegin
CREATE FUNCTION payouts.check_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.user_id, NEW.kind, NEW.route, NEW.gross_kes, NEW.tax_kes, NEW.net_kes, NEW.msisdn, NEW.draw_result_id,
      NEW.originator_conversation_id, NEW.idempotency_key)
     IS DISTINCT FROM
     (OLD.user_id, OLD.kind, OLD.route, OLD.gross_kes, OLD.tax_kes, OLD.net_kes, OLD.msisdn, OLD.draw_result_id,
      OLD.originator_conversation_id, OLD.idempotency_key) THEN
    RAISE EXCEPTION 'payout %: amounts, payee and keys cannot change', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF OLD.status IN ('succeeded','failed') THEN
    RAISE EXCEPTION 'payout %: a % payout is final', OLD.id, OLD.status USING ERRCODE = 'restrict_violation';
  END IF;
  IF NEW.status <> OLD.status AND (OLD.status, NEW.status) NOT IN (
      ('held','pending'), ('held','failed'),
      ('pending','sending'), ('sending','sent'), ('sending','timed_out'), ('sending','failed'),
      ('sent','result_received'), ('sent','succeeded'), ('sent','failed'), ('sent','manual'),
      ('timed_out','result_received'), ('timed_out','succeeded'), ('timed_out','failed'), ('timed_out','manual'),
      ('result_received','succeeded'), ('result_received','failed'), ('result_received','manual'),
      ('manual','succeeded'), ('manual','failed')) THEN
    RAISE EXCEPTION 'payout %: % -> % is not allowed', OLD.id, OLD.status, NEW.status USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER payouts_transition BEFORE UPDATE ON payouts.payouts
  FOR EACH ROW EXECUTE FUNCTION payouts.check_transition();
CREATE TRIGGER payouts_no_delete BEFORE DELETE ON payouts.payouts
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- What Daraja posts about payouts: payment results and Transaction Status answers, kept as received. The api only
-- appends them (it may not change payouts); the worker settles payouts from them.
CREATE TABLE payouts.b2c_callbacks (
  id           uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  kind         text NOT NULL CHECK (kind IN ('result','timeout','status_result','status_timeout')),
  payout_id    uuid REFERENCES payouts.payouts (id),
  originator   text,
  raw          jsonb NOT NULL,
  received_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX b2c_callbacks_payout_idx ON payouts.b2c_callbacks (payout_id, received_at);
CREATE TRIGGER b2c_callbacks_append_only BEFORE UPDATE OR DELETE ON payouts.b2c_callbacks
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- PLACEHOLDERS until the owner and a tax adviser confirm them (docs/checkpoints/phase-5.md): winnings are taxed 20%
-- when a prize is paid; withdrawals of the wallet (the player's deposits) are not taxed.
INSERT INTO ledger.tax_rules (code, rate, point_of_taxation, rounding, effective_from, created_by) VALUES
  ('winnings_wht',   0.20, 'prize_payout', 'half_up', '2000-01-01', 'migration:placeholder'),
  ('withdrawal_wht', 0,    'withdrawal',   'half_up', '2000-01-01', 'migration:placeholder');

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON payouts.b2c_callbacks TO app_api, app_worker;
  GRANT SELECT ON payouts.b2c_callbacks TO app_readonly;
  ALTER TABLE payouts.b2c_callbacks OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE payouts.b2c_callbacks;
DELETE FROM ledger.tax_rules WHERE code IN ('winnings_wht','withdrawal_wht') AND created_by = 'migration:placeholder';
DROP TRIGGER payouts_no_delete ON payouts.payouts;
DROP TRIGGER payouts_transition ON payouts.payouts;
DROP FUNCTION payouts.check_transition();
DROP INDEX payouts.payouts_draw_result_once;
CREATE UNIQUE INDEX payouts_draw_result_once ON payouts.payouts (draw_result_id) WHERE draw_result_id IS NOT NULL;
DROP INDEX payouts.payouts_open_idx;
DROP INDEX payouts.payouts_user_created_idx;
ALTER TABLE payouts.payouts DROP CONSTRAINT payouts_prize_has_result, DROP CONSTRAINT payouts_final_is_settled,
  DROP CONSTRAINT payouts_succeeded_has_receipt, DROP CONSTRAINT payouts_b2c_needs_msisdn,
  DROP COLUMN settled_at, DROP COLUMN settled_by, DROP COLUMN approved_by, DROP COLUMN reason, DROP COLUMN status_originator,
  DROP COLUMN status_requested_at, DROP COLUMN status_requests, DROP COLUMN tax_rounding, DROP COLUMN tax_rate, DROP COLUMN tax_code,
  DROP CONSTRAINT payouts_status_check;
UPDATE payouts.payouts SET status = 'pending' WHERE status IN ('held','sending','result_received');
ALTER TABLE payouts.payouts ADD CONSTRAINT payouts_status_check
  CHECK (status IN ('pending','sent','timed_out','succeeded','failed','manual'));
