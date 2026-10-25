-- +goose Up
-- A prize first repays what its winner owes from a reversed top-up (docs/checkpoints/phase-5.md): the winnings tax is
-- withheld on the whole prize, the debt is offset from what is left, and only the rest is sent through B2C. A prize
-- the debt absorbs entirely sends nothing: it is recorded on the 'offset' route, settled when it is created.
ALTER TABLE payouts.payouts
  ADD COLUMN debt_offset_kes bigint NOT NULL DEFAULT 0 CHECK (debt_offset_kes >= 0),
  DROP CONSTRAINT payouts_net_is_gross_minus_tax,
  DROP CONSTRAINT payouts_net_kes_check,
  DROP CONSTRAINT payouts_route_check,
  DROP CONSTRAINT payouts_succeeded_has_receipt,
  ADD CONSTRAINT payouts_net_is_gross_minus_tax_and_offset CHECK (net_kes = gross_kes - tax_kes - debt_offset_kes),
  ADD CONSTRAINT payouts_net_kes_check CHECK (net_kes >= 0),
  ADD CONSTRAINT payouts_route_check CHECK (route IN ('b2c','bank','offset')),
  -- only a prize repays debt, and only the offset route sends nothing
  ADD CONSTRAINT payouts_offset_only_prizes CHECK (debt_offset_kes = 0 OR kind = 'prize'),
  ADD CONSTRAINT payouts_offset_route CHECK ((route = 'offset') = (net_kes = 0)),
  ADD CONSTRAINT payouts_offset_route_settled CHECK (route <> 'offset' OR status = 'succeeded'),
  ADD CONSTRAINT payouts_succeeded_has_receipt CHECK (status <> 'succeeded' OR route = 'offset' OR mpesa_receipt IS NOT NULL);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION payouts.check_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.user_id, NEW.kind, NEW.route, NEW.gross_kes, NEW.tax_kes, NEW.net_kes, NEW.debt_offset_kes, NEW.msisdn,
      NEW.draw_result_id, NEW.originator_conversation_id, NEW.idempotency_key)
     IS DISTINCT FROM
     (OLD.user_id, OLD.kind, OLD.route, OLD.gross_kes, OLD.tax_kes, OLD.net_kes, OLD.debt_offset_kes, OLD.msisdn,
      OLD.draw_result_id, OLD.originator_conversation_id, OLD.idempotency_key) THEN
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

-- +goose Down
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
ALTER TABLE payouts.payouts
  DROP CONSTRAINT payouts_succeeded_has_receipt,
  DROP CONSTRAINT payouts_offset_route_settled,
  DROP CONSTRAINT payouts_offset_route,
  DROP CONSTRAINT payouts_offset_only_prizes,
  DROP CONSTRAINT payouts_route_check,
  DROP CONSTRAINT payouts_net_kes_check,
  DROP CONSTRAINT payouts_net_is_gross_minus_tax_and_offset,
  ADD CONSTRAINT payouts_succeeded_has_receipt CHECK (status <> 'succeeded' OR mpesa_receipt IS NOT NULL),
  ADD CONSTRAINT payouts_route_check CHECK (route IN ('b2c','bank')),
  ADD CONSTRAINT payouts_net_kes_check CHECK (net_kes > 0),
  ADD CONSTRAINT payouts_net_is_gross_minus_tax CHECK (net_kes = gross_kes - tax_kes),
  DROP COLUMN debt_offset_kes;
