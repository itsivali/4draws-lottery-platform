-- +goose Up
-- Owner, 2026-10-31: M-Pesa payments that went through but were never credited (the callback never came, the status
-- query was throttled, the sandbox lied) are reconciled from the receipt: the player's own M-Pesa message, the
-- M-Pesa statement, or Safaricom's Transaction Status answer. A payment carries the receipt the player claims and
-- what became of the claim; every statement import is kept with what it matched.
ALTER TABLE payments.payments
  ADD COLUMN claimed_receipt  text CHECK (claimed_receipt ~ '^[A-Z0-9]{10}$'),
  ADD COLUMN claimed_at       timestamptz,
  ADD COLUMN claim_status     text CHECK (claim_status IN ('claimed','checking','confirmed','unverified','rejected')),
  ADD COLUMN claim_originator text, -- Transaction Status request that asks M-Pesa about the claimed receipt
  ADD COLUMN claim_result     jsonb, -- M-Pesa's answer, as posted
  ADD COLUMN claim_note       text,  -- why a claim could not be confirmed, or an operator's note
  ADD CONSTRAINT payments_claim_together CHECK ((claimed_receipt IS NULL) = (claim_status IS NULL) AND (claimed_receipt IS NULL) = (claimed_at IS NULL));
CREATE UNIQUE INDEX payments_claim_originator_key ON payments.payments (claim_originator) WHERE claim_originator IS NOT NULL;
CREATE INDEX payments_claims_open_idx ON payments.payments (claimed_at) WHERE claim_status IN ('claimed','checking','unverified');

-- An M-Pesa statement an operator imported, with the matching report it produced and what was credited from it.
CREATE TABLE payments.statement_imports (
  id            uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  imported_by   text NOT NULL CHECK (imported_by <> ''),
  imported_at   timestamptz NOT NULL DEFAULT now(),
  filename      text NOT NULL,
  sha256        text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  rows_total    integer NOT NULL CHECK (rows_total >= 0),
  matched       integer NOT NULL CHECK (matched >= 0),
  credited      integer NOT NULL CHECK (credited >= 0),
  credited_kes  bigint NOT NULL CHECK (credited_kes >= 0),
  report        jsonb NOT NULL
);
CREATE INDEX statement_imports_at_idx ON payments.statement_imports (imported_at DESC);
CREATE TRIGGER statement_imports_append_only BEFORE UPDATE OR DELETE ON payments.statement_imports
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON payments.statement_imports TO app_api, app_worker;
  GRANT SELECT ON payments.statement_imports TO app_readonly;
  ALTER TABLE payments.statement_imports OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE payments.statement_imports;
DROP INDEX payments.payments_claims_open_idx;
DROP INDEX payments.payments_claim_originator_key;
ALTER TABLE payments.payments
  DROP CONSTRAINT payments_claim_together,
  DROP COLUMN claimed_receipt, DROP COLUMN claimed_at, DROP COLUMN claim_status, DROP COLUMN claim_originator,
  DROP COLUMN claim_result, DROP COLUMN claim_note;
