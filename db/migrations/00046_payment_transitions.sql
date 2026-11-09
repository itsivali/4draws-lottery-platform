-- +goose Up
-- ADR 0028: a top-up only moves forward, and what M-Pesa said about it is written once. The first callback is kept
-- (a later one cannot replace the amount or receipt it carries), a credited top-up keeps its amount and receipt, and
-- money once credited leaves only by a reversal.
-- +goose StatementBegin
CREATE FUNCTION payments.check_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.user_id, NEW.channel, NEW.amount_requested_kes, NEW.msisdn, NEW.idempotency_key, NEW.created_at)
     IS DISTINCT FROM
     (OLD.user_id, OLD.channel, OLD.amount_requested_kes, OLD.msisdn, OLD.idempotency_key, OLD.created_at) THEN
    RAISE EXCEPTION 'payment %: the player, channel, amount asked for and number cannot change', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF (OLD.merchant_request_id IS NOT NULL AND NEW.merchant_request_id IS DISTINCT FROM OLD.merchant_request_id)
     OR (OLD.checkout_request_id IS NOT NULL AND NEW.checkout_request_id IS DISTINCT FROM OLD.checkout_request_id) THEN
    RAISE EXCEPTION 'payment %: the prompt''s ids cannot change', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF OLD.raw_callback IS NOT NULL AND NEW.raw_callback IS DISTINCT FROM OLD.raw_callback THEN
    RAISE EXCEPTION 'payment %: the first callback is kept', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF (OLD.amount_confirmed_kes IS NOT NULL AND NEW.amount_confirmed_kes IS DISTINCT FROM OLD.amount_confirmed_kes)
     OR (OLD.mpesa_receipt IS NOT NULL AND NEW.mpesa_receipt IS DISTINCT FROM OLD.mpesa_receipt) THEN
    RAISE EXCEPTION 'payment %: a confirmed amount and receipt cannot change', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF NEW.status = OLD.status THEN
    RETURN NEW;
  END IF;
  -- reversed is final; credited is left only by a reversal; pending states only move forward; the uncredited final
  -- states (failed, cancelled, expired, unconfirmed) may still be corrected or credited when M-Pesa shows otherwise
  IF OLD.status = 'reversed'
     OR (OLD.status = 'credited' AND NEW.status <> 'reversed')
     OR (NEW.status = 'reversed' AND OLD.status <> 'credited')
     OR (NEW.status = 'initiated')
     OR (NEW.status = 'stk_sent' AND OLD.status <> 'initiated')
     OR (NEW.status = 'callback_received' AND OLD.status NOT IN ('initiated','stk_sent'))
     OR (NEW.status = 'verified' AND OLD.status NOT IN ('initiated','stk_sent','callback_received')) THEN
    RAISE EXCEPTION 'payment %: % -> % is not allowed', OLD.id, OLD.status, NEW.status USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER payments_transition BEFORE UPDATE ON payments.payments
  FOR EACH ROW EXECUTE FUNCTION payments.check_transition();
CREATE TRIGGER payments_no_delete BEFORE DELETE ON payments.payments
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose Down
DROP TRIGGER payments_no_delete ON payments.payments;
DROP TRIGGER payments_transition ON payments.payments;
DROP FUNCTION payments.check_transition();
