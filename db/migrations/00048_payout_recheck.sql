-- +goose Up
-- ADR 0030: a payout is sent only to an account that may still be paid. One that cannot be paid when its turn comes
-- (suspended, closed, no longer verified, or its proved number changed) goes back to held for an operator, instead of
-- being sent: pending -> held is the one new move. Nothing else changes.
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
