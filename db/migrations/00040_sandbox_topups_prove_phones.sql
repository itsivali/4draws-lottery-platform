-- +goose Up
-- Owner, 2026-10-30: a sandbox-settled top-up proves the account's number (#103), as an approved prompt does. Top-ups
-- settled between #102 and #103 credited without proving it, so those accounts could not play. This proves the
-- number for each, still from a top-up: the earliest credited SANDBOX- top-up per number, sent to the account's
-- current number, when no account has that number proved already. Nothing to do on a database that never ran
-- sandbox settlement.
WITH picked AS (
  SELECT DISTINCT ON (p.msisdn) p.user_id
  FROM payments.payments p
  JOIN identity.users u ON u.id = p.user_id
  WHERE p.channel = 'stk' AND p.status = 'credited' AND p.mpesa_receipt LIKE 'SANDBOX-%'
    AND u.msisdn = p.msisdn AND u.msisdn_verified_at IS NULL
    AND NOT EXISTS (SELECT 1 FROM identity.users o WHERE o.msisdn = p.msisdn AND o.msisdn_verified_at IS NOT NULL)
  ORDER BY p.msisdn, p.updated_at
), proved AS (
  UPDATE identity.users u SET msisdn_verified_at = now(), updated_at = now()
  FROM picked WHERE u.id = picked.user_id
  RETURNING u.id
)
INSERT INTO audit.audit_log (actor_type, action, entity_type, entity_id, details)
SELECT 'system', 'user.phone_verified', 'user', id::text, '{"source": "sandbox-settled top-up (migration 00040)"}'::jsonb
FROM proved;

-- +goose Down
-- Proofs are not taken back: the numbers were proved by top-ups.
SELECT 1;
