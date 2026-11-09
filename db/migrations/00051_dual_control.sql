-- +goose Up
-- ADR 0032: no one person releases or settles a payout.
-- A held payout is sent once two different staff members approved it (a refund's requester counts as the first); each
-- approval is kept. A payout whose outcome could not be confirmed (manual) is settled when a second staff member
-- confirms exactly what a first proposed: paid with that receipt, or not paid. Proposals are kept, the latest open.
CREATE TABLE payouts.approvals (
  payout_id   uuid NOT NULL REFERENCES payouts.payouts (id),
  staff       text NOT NULL CHECK (staff ~ '^staff:'),
  approved_at timestamptz NOT NULL,
  PRIMARY KEY (payout_id, staff)
);
CREATE TRIGGER approvals_append_only BEFORE UPDATE OR DELETE ON payouts.approvals
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

CREATE TABLE payouts.settlement_proposals (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  payout_id   uuid NOT NULL REFERENCES payouts.payouts (id),
  paid        boolean NOT NULL,
  receipt     text,
  reason      text,
  staff       text NOT NULL CHECK (staff ~ '^staff:'),
  proposed_at timestamptz NOT NULL,
  CONSTRAINT settlement_proposals_paid_has_receipt CHECK (NOT paid OR receipt IS NOT NULL),
  CONSTRAINT settlement_proposals_unpaid_has_reason CHECK (paid OR reason IS NOT NULL)
);
CREATE INDEX settlement_proposals_payout_idx ON payouts.settlement_proposals (payout_id, id);
CREATE TRIGGER settlement_proposals_append_only BEFORE UPDATE OR DELETE ON payouts.settlement_proposals
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON payouts.approvals, payouts.settlement_proposals TO app_api, app_worker;
  GRANT SELECT ON payouts.approvals, payouts.settlement_proposals TO app_readonly;
  ALTER TABLE payouts.approvals OWNER TO app_migrator;
  ALTER TABLE payouts.settlement_proposals OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE payouts.settlement_proposals;
DROP TABLE payouts.approvals;
