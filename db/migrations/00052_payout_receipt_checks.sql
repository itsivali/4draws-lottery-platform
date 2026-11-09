-- +goose Up
-- ADR 0034: a payout whose outcome could not be confirmed (manual) is settled as paid only when M-Pesa confirms the
-- receipt staff found on the statement. Each claimed receipt is a check: the receipt, who claimed it, the Transaction
-- Status request that asks M-Pesa about it, and M-Pesa's answer. A check is decided once and never changes after.
CREATE TABLE payouts.receipt_checks (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  payout_id         uuid NOT NULL REFERENCES payouts.payouts (id),
  receipt           text NOT NULL CHECK (receipt ~ '^[A-Z0-9]{10}$'),
  staff             text NOT NULL CHECK (staff ~ '^staff:'),
  status_originator text,   -- our Transaction Status request, once M-Pesa accepted it
  requested_at      timestamptz NOT NULL,
  outcome           text CHECK (outcome IN ('confirmed','refuted','no_answer')),
  detail            text,
  decided_at        timestamptz,
  CONSTRAINT receipt_checks_decided CHECK ((outcome IS NULL) = (decided_at IS NULL))
);
CREATE INDEX receipt_checks_payout_idx ON payouts.receipt_checks (payout_id, id);
CREATE UNIQUE INDEX receipt_checks_originator_key ON payouts.receipt_checks (status_originator) WHERE status_originator IS NOT NULL;

-- the request id is set once, the answer once; nothing else changes and nothing is deleted
-- +goose StatementBegin
CREATE FUNCTION payouts.check_receipt_check() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.payout_id, NEW.receipt, NEW.staff, NEW.requested_at)
     IS DISTINCT FROM (OLD.id, OLD.payout_id, OLD.receipt, OLD.staff, OLD.requested_at)
     OR (OLD.status_originator IS NOT NULL AND NEW.status_originator IS DISTINCT FROM OLD.status_originator)
     OR (OLD.outcome IS NOT NULL AND (NEW.outcome, NEW.detail, NEW.decided_at) IS DISTINCT FROM (OLD.outcome, OLD.detail, OLD.decided_at)) THEN
    RAISE EXCEPTION 'receipt check %: only its request and its answer may be recorded, once', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER receipt_checks_check BEFORE UPDATE ON payouts.receipt_checks
  FOR EACH ROW EXECUTE FUNCTION payouts.check_receipt_check();
CREATE TRIGGER receipt_checks_no_delete BEFORE DELETE ON payouts.receipt_checks
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT, UPDATE ON payouts.receipt_checks TO app_api, app_worker;
  GRANT SELECT ON payouts.receipt_checks TO app_readonly;
  ALTER TABLE payouts.receipt_checks OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE payouts.receipt_checks;
DROP FUNCTION payouts.check_receipt_check();
