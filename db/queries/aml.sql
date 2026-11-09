-- name: LatestRuleSettings :many
-- The setting in force for each rule that has one (ADR 0033).
SELECT DISTINCT ON (rule) rule, enabled, params, approval_reference, set_by, note, set_at
FROM compliance.aml_rule_settings ORDER BY rule, id DESC;

-- name: InsertRuleSetting :exec
INSERT INTO compliance.aml_rule_settings (rule, enabled, params, approval_reference, set_by, note, set_at)
VALUES (sqlc.arg(rule), sqlc.arg(enabled), sqlc.arg(params), sqlc.narg(approval_reference), sqlc.arg(set_by), sqlc.arg(note), sqlc.arg(at));

-- name: DepositVelocity :many
-- Players whose credited top-ups since the window start exceed the amount or the count.
SELECT user_id::text AS user_id, sum(amount_confirmed_kes)::bigint AS deposited_kes, count(*)::bigint AS deposits
FROM payments.payments
WHERE status IN ('credited','reversed') AND created_at >= sqlc.arg(since)
GROUP BY user_id
HAVING sum(amount_confirmed_kes) > sqlc.arg(max_kes)::bigint OR count(*) > sqlc.arg(max_count)::bigint;

-- name: InOutLittlePlay :many
-- Players who withdrew at least the minimum since the window start while staking less than a share of what they
-- deposited in it (refunds to source excluded: staff sent those).
WITH w AS (
  SELECT po.user_id, sum(po.gross_kes)::bigint AS withdrawn FROM payouts.payouts po
  WHERE po.kind = 'withdrawal' AND po.status <> 'failed' AND po.refund_reason IS NULL AND po.created_at >= sqlc.arg(since)::timestamptz
  GROUP BY po.user_id),
d AS (
  SELECT wd.user_id, sum(wd.amount_kes)::bigint AS deposited FROM wallet.wallet_transactions wd
  WHERE wd.kind = 'deposit' AND wd.created_at >= sqlc.arg(since)::timestamptz GROUP BY wd.user_id),
s AS (
  SELECT ws.user_id, (-sum(ws.amount_kes))::bigint AS staked FROM wallet.wallet_transactions ws
  WHERE ws.kind = 'stake' AND ws.created_at >= sqlc.arg(since)::timestamptz GROUP BY ws.user_id)
SELECT w.user_id::text AS user_id, d.deposited AS deposited_kes, coalesce(s.staked, 0)::bigint AS staked_kes, w.withdrawn AS withdrawn_kes
FROM w JOIN d ON d.user_id = w.user_id LEFT JOIN s ON s.user_id = w.user_id
WHERE w.withdrawn >= sqlc.arg(min_withdrawal_kes)::bigint
  AND coalesce(s.staked, 0) * 100 < d.deposited * sqlc.arg(max_play_percent)::bigint;

-- name: FailedTopUps :many
-- Players with at least the minimum of top-ups that failed, were cancelled or expired since the window start.
SELECT user_id::text AS user_id, count(*)::bigint AS failed
FROM payments.payments
WHERE status IN ('failed','cancelled','expired') AND created_at >= sqlc.arg(since)
GROUP BY user_id HAVING count(*) >= sqlc.arg(min_failed)::bigint;

-- name: SharedIDNumbers :many
-- Players whose declared ID number at least one other account also declared (a group of at least min_accounts).
SELECT d.user_id::text AS user_id,
       (SELECT array_agg(o.user_id::text ORDER BY o.user_id) FROM kyc.declared_identities o
        WHERE o.id_number_hash = d.id_number_hash AND o.user_id <> d.user_id)::text[] AS other_accounts
FROM kyc.declared_identities d
WHERE (SELECT count(*) FROM kyc.declared_identities o WHERE o.id_number_hash = d.id_number_hash) >= sqlc.arg(min_accounts)::bigint;

-- name: SharedDevices :many
-- Players who signed in on a device (by its fingerprint) that at least min_accounts accounts used.
SELECT DISTINCT d.user_id::text AS user_id,
       (SELECT array_agg(DISTINCT o.user_id::text) FROM identity.devices o
        WHERE o.fingerprint_hash = d.fingerprint_hash AND o.user_id <> d.user_id)::text[] AS other_accounts
FROM identity.devices d
WHERE (SELECT count(DISTINCT o.user_id) FROM identity.devices o WHERE o.fingerprint_hash = d.fingerprint_hash) >= sqlc.arg(min_accounts)::bigint;

-- name: OpenCase :one
-- Opens a case unless the player has an open one for the rule, or one closed within the quiet period.
INSERT INTO compliance.aml_cases (user_id, rule, evidence, opened_by, opened_at, updated_at)
SELECT sqlc.arg(user_id)::uuid, sqlc.arg(rule), sqlc.arg(evidence), sqlc.narg(opened_by), sqlc.arg(at), sqlc.arg(at)
WHERE NOT EXISTS (
  SELECT 1 FROM compliance.aml_cases c WHERE c.user_id = sqlc.arg(user_id)::uuid AND c.rule = sqlc.arg(rule)
    AND (c.status = 'open' OR c.closed_at > sqlc.arg(quiet_since)::timestamptz))
ON CONFLICT (user_id, rule) WHERE status = 'open' DO NOTHING
RETURNING id;

-- name: InsertCaseEvent :exec
INSERT INTO compliance.aml_case_events (case_id, action, staff, note, at)
VALUES (sqlc.arg(case_id), sqlc.arg(action), sqlc.narg(staff), sqlc.narg(note), sqlc.arg(at));

-- name: LockCase :one
SELECT * FROM compliance.aml_cases WHERE id = $1 FOR UPDATE;

-- name: GetCase :one
SELECT * FROM compliance.aml_cases WHERE id = $1;

-- name: CaseEvents :many
SELECT action, staff, note, at FROM compliance.aml_case_events WHERE case_id = $1 ORDER BY id;

-- name: SetHold :exec
UPDATE compliance.aml_cases SET hold = sqlc.arg(hold), hold_by = sqlc.narg(hold_by), updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: CloseCase :exec
UPDATE compliance.aml_cases SET status = 'closed', hold = false, outcome = sqlc.arg(outcome), closed_at = sqlc.arg(at),
  closed_by = sqlc.arg(closed_by), updated_at = sqlc.arg(at)
WHERE id = sqlc.arg(id);

-- name: TouchCase :exec
UPDATE compliance.aml_cases SET updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: Cases :many
-- Cases in a view (open, held or closed), oldest first, after a cursor (a case id).
SELECT c.*, u.msisdn FROM compliance.aml_cases c JOIN identity.users u ON u.id = c.user_id
WHERE CASE sqlc.arg(view)::text WHEN 'held' THEN c.hold WHEN 'closed' THEN c.status = 'closed' ELSE c.status = 'open' END
  AND (sqlc.narg(after)::uuid IS NULL OR c.id > sqlc.narg(after)::uuid)
ORDER BY c.id LIMIT sqlc.arg(max_rows);

-- name: OnHold :one
-- Whether an open case holds the player's money.
SELECT EXISTS (SELECT 1 FROM compliance.aml_cases WHERE user_id = $1 AND hold)::boolean;
