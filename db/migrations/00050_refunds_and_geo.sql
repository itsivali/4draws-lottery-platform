-- +goose Up
-- ADR 0031.
-- Refund to source: a balance the account cannot withdraw (it may not be paid: suspended, identity refused or never
-- verified, phone not proved) goes back to the number that paid it in, as a withdrawal that an admin asks for with a
-- reason and a second staff member approves. The refund's reason and requester are part of what a payout is, and
-- never change.
ALTER TABLE payouts.payouts
  ADD COLUMN refund_reason       text,
  ADD COLUMN refund_requested_by text,
  ADD CONSTRAINT payouts_refund_named CHECK ((refund_reason IS NULL) = (refund_requested_by IS NULL)),
  ADD CONSTRAINT payouts_refund_is_withdrawal CHECK (refund_reason IS NULL OR kind = 'withdrawal');

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION payouts.check_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.user_id, NEW.kind, NEW.route, NEW.gross_kes, NEW.tax_kes, NEW.net_kes, NEW.msisdn, NEW.draw_result_id,
      NEW.originator_conversation_id, NEW.idempotency_key, NEW.refund_reason, NEW.refund_requested_by)
     IS DISTINCT FROM
     (OLD.user_id, OLD.kind, OLD.route, OLD.gross_kes, OLD.tax_kes, OLD.net_kes, OLD.msisdn, OLD.draw_result_id,
      OLD.originator_conversation_id, OLD.idempotency_key, OLD.refund_reason, OLD.refund_requested_by) THEN
    RAISE EXCEPTION 'payout %: amounts, payee and keys cannot change', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF OLD.status IN ('succeeded','failed') THEN
    RAISE EXCEPTION 'payout %: a % payout is final', OLD.id, OLD.status USING ERRCODE = 'restrict_violation';
  END IF;
  IF NEW.status <> OLD.status AND (OLD.status, NEW.status) NOT IN (
      ('held','pending'), ('held','failed'),
      ('pending','sending'), ('pending','held'), ('sending','sent'), ('sending','timed_out'), ('sending','failed'),
      ('sent','result_received'), ('sent','succeeded'), ('sent','failed'), ('sent','manual'),
      ('timed_out','result_received'), ('timed_out','succeeded'), ('timed_out','failed'), ('timed_out','manual'),
      ('result_received','succeeded'), ('result_received','failed'), ('result_received','manual'),
      ('manual','succeeded'), ('manual','failed')) THEN
    RAISE EXCEPTION 'payout %: % -> % is not allowed', OLD.id, OLD.status, NEW.status USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- Where a player is when they sign up and move money: the country the edge (Cloudflare's CF-IPCountry) reports. A
-- risk signal, not proof of location; with real money anything but KE is refused. Never changed or deleted.
CREATE TABLE compliance.geo_signals (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     uuid REFERENCES identity.users (id),
  action      text NOT NULL CHECK (action IN ('signup','topup','play','withdrawal')),
  country     text CHECK (country ~ '^[A-Z0-9]{2}$'),
  refused     boolean NOT NULL,
  observed_at timestamptz NOT NULL
);
CREATE INDEX geo_signals_user_idx ON compliance.geo_signals (user_id, id);
CREATE TRIGGER geo_signals_append_only BEFORE UPDATE OR DELETE ON compliance.geo_signals
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON compliance.geo_signals TO app_api, app_worker;
  GRANT SELECT ON compliance.geo_signals TO app_readonly;
  ALTER TABLE compliance.geo_signals OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE compliance.geo_signals;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION payouts.check_transition() RETURNS trigger
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
      ('pending','sending'), ('pending','held'), ('sending','sent'), ('sending','timed_out'), ('sending','failed'),
      ('sent','result_received'), ('sent','succeeded'), ('sent','failed'), ('sent','manual'),
      ('timed_out','result_received'), ('timed_out','succeeded'), ('timed_out','failed'), ('timed_out','manual'),
      ('result_received','succeeded'), ('result_received','failed'), ('result_received','manual'),
      ('manual','succeeded'), ('manual','failed')) THEN
    RAISE EXCEPTION 'payout %: % -> % is not allowed', OLD.id, OLD.status, NEW.status USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
ALTER TABLE payouts.payouts DROP COLUMN refund_requested_by, DROP COLUMN refund_reason;
