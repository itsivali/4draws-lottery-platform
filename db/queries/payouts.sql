-- name: InsertPayout :one
INSERT INTO payouts.payouts (user_id, kind, route, status, gross_kes, tax_kes, debt_offset_kes, net_kes, msisdn, draw_result_id,
                             originator_conversation_id, idempotency_key, tax_code, tax_rate, tax_rounding, settled_by, settled_at,
                             created_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(kind), sqlc.arg(route), sqlc.arg(status), sqlc.arg(gross_kes), sqlc.arg(tax_kes),
        sqlc.arg(debt_offset_kes), sqlc.arg(net_kes), sqlc.arg(msisdn), sqlc.narg(draw_result_id), platform.uuid_v7()::text,
        sqlc.arg(idempotency_key), sqlc.arg(tax_code), (sqlc.arg(tax_rate)::text)::numeric, sqlc.arg(tax_rounding),
        sqlc.narg(settled_by), sqlc.narg(settled_at), sqlc.arg(at), sqlc.arg(at))
RETURNING id;

-- name: PayoutByKey :one
SELECT id FROM payouts.payouts WHERE user_id = $1 AND idempotency_key = $2;

-- name: GetPayout :one
SELECT * FROM payouts.payouts WHERE id = $1;

-- name: LockPayout :one
SELECT * FROM payouts.payouts WHERE id = $1 FOR UPDATE;

-- name: PayoutByOriginator :one
SELECT id FROM payouts.payouts WHERE originator_conversation_id = $1;

-- name: PayoutByStatusOriginator :one
SELECT id FROM payouts.payouts WHERE status_originator = $1;

-- name: WithdrawalsSince :one
-- withdrawals that count against the daily limit: every one not refused or failed
SELECT count(*)::int FROM payouts.payouts
WHERE user_id = $1 AND kind = 'withdrawal' AND created_at >= sqlc.arg(since) AND status <> 'failed';

-- name: PlayerPayouts :many
SELECT * FROM payouts.payouts WHERE user_id = $1 ORDER BY created_at DESC LIMIT sqlc.arg(max_rows);

-- name: PayoutsInStatus :many
SELECT * FROM payouts.payouts WHERE status = $1 ORDER BY created_at LIMIT sqlc.arg(max_rows);

-- name: StalePayouts :many
-- payouts the worker should look at again: still open and untouched for a while
SELECT id, status FROM payouts.payouts
WHERE status IN ('pending','sending','sent','timed_out','result_received') AND updated_at < sqlc.arg(before)
ORDER BY updated_at LIMIT 200;

-- name: MarkSent :exec
UPDATE payouts.payouts SET status = 'sent', conversation_id = sqlc.arg(conversation_id), updated_at = sqlc.arg(at)
WHERE id = sqlc.arg(id) AND status = 'sending';

-- name: SetPayoutStatus :exec
UPDATE payouts.payouts SET status = sqlc.arg(status), reason = sqlc.narg(reason), updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: MarkStatusRequested :exec
UPDATE payouts.payouts
   SET status_requests = status_requests + 1, status_requested_at = sqlc.arg(at), status_originator = sqlc.arg(originator),
       updated_at = sqlc.arg(at)
 WHERE id = sqlc.arg(id);

-- name: SettlePayout :exec
UPDATE payouts.payouts
   SET status = sqlc.arg(status), mpesa_receipt = sqlc.narg(receipt), reason = sqlc.narg(reason), settled_by = sqlc.narg(settled_by),
       settled_at = sqlc.arg(at), updated_at = sqlc.arg(at)
 WHERE id = sqlc.arg(id);

-- name: ApprovePayout :exec
UPDATE payouts.payouts SET status = 'pending', approved_by = sqlc.arg(approved_by), updated_at = sqlc.arg(at)
WHERE id = sqlc.arg(id) AND status = 'held';

-- name: InsertCallback :exec
INSERT INTO payouts.b2c_callbacks (kind, payout_id, originator, raw, received_at)
VALUES (sqlc.arg(kind), sqlc.narg(payout_id), sqlc.narg(originator), sqlc.arg(raw), sqlc.arg(at));

-- name: TaxRateText :one
SELECT coalesce(tax_rate::text, '0')::text FROM payouts.payouts WHERE id = $1;

-- name: Callbacks :many
SELECT kind, originator, raw, received_at FROM payouts.b2c_callbacks WHERE payout_id = $1 ORDER BY received_at, id;

-- name: ReceiptPaid :one
SELECT EXISTS (SELECT 1 FROM payouts.payouts WHERE mpesa_receipt = $1)::bool;

-- name: PrizesToPay :many
-- results of approved draws that have never had a payout (a failed one is retried by an operator, not here)
SELECT r.id AS result_id, e.user_id, r.amount_kes, d.draw_type, d.id AS draw_id
FROM draws.draw_results r
JOIN draws.draw_entries e ON e.id = r.entry_id
JOIN draws.draws d ON d.id = r.draw_id
WHERE d.status = 'approved' AND NOT EXISTS (SELECT 1 FROM payouts.payouts p WHERE p.draw_result_id = r.id)
ORDER BY d.period_end, r.prize_rank
LIMIT 500;

-- name: PrizeResult :one
SELECT r.id AS result_id, e.user_id, r.amount_kes, d.draw_type, d.id AS draw_id, d.status AS draw_status
FROM draws.draw_results r
JOIN draws.draw_entries e ON e.id = r.entry_id
JOIN draws.draws d ON d.id = r.draw_id
WHERE r.id = $1;

-- name: ResultPayouts :one
-- how many payouts a prize has had, and whether one is live (not failed)
SELECT count(*)::int AS attempts, count(*) FILTER (WHERE status <> 'failed')::int AS live
FROM payouts.payouts WHERE draw_result_id = $1;

-- name: InsertKRAEvent :exec
INSERT INTO audit.kra_outbox (event_type, payload, created_at) VALUES (sqlc.arg(event_type), sqlc.arg(payload), sqlc.arg(at))
ON CONFLICT DO NOTHING;

-- name: PendingKRAEvents :many
SELECT id, event_type, payload, created_at FROM audit.kra_outbox WHERE exported_at IS NULL ORDER BY id LIMIT sqlc.arg(max_rows) FOR UPDATE;

-- name: MarkKRAExported :exec
UPDATE audit.kra_outbox SET exported_at = sqlc.arg(at) WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: PayoutsWithoutJournal :many
SELECT p.id FROM payouts.payouts p
WHERE NOT EXISTS (SELECT 1 FROM ledger.ledger_journals j WHERE j.reference_type = 'payout' AND j.reference_id = p.id
                  AND j.kind = CASE p.kind WHEN 'prize' THEN 'prize_payout' ELSE 'withdrawal' END);

-- name: FailureJournalMismatch :many
-- a failed payout without its reversal, or a reversal of a payout that did not fail
SELECT p.id FROM payouts.payouts p
WHERE (p.status = 'failed') <> EXISTS (SELECT 1 FROM ledger.ledger_journals j WHERE j.reference_type = 'payout' AND j.reference_id = p.id
                  AND j.kind IN ('withdrawal_failed','prize_payout_failed'));

-- name: JournalAmountMismatch :many
-- the payout's journal must move exactly its net to B2C_FLOAT, its tax to TAX_PAYABLE and its debt offset off PLAYER_DEBT
SELECT p.id FROM payouts.payouts p
JOIN ledger.ledger_journals j ON j.reference_type = 'payout' AND j.reference_id = p.id AND j.kind IN ('withdrawal','prize_payout')
WHERE coalesce((SELECT sum(e.amount_kes) FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id
                WHERE e.journal_id = j.id AND a.code = 'B2C_FLOAT' AND e.direction = 'C'), 0) <> p.net_kes
   OR coalesce((SELECT sum(e.amount_kes) FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id
                WHERE e.journal_id = j.id AND a.code = 'TAX_PAYABLE' AND e.direction = 'C'), 0) <> p.tax_kes
   OR coalesce((SELECT sum(e.amount_kes) FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id
                WHERE e.journal_id = j.id AND a.code = 'PLAYER_DEBT' AND e.direction = 'C' AND e.user_id = p.user_id), 0) <> p.debt_offset_kes;

-- name: PrizeAmountMismatch :many
SELECT p.id FROM payouts.payouts p JOIN draws.draw_results r ON r.id = p.draw_result_id WHERE p.gross_kes <> r.amount_kes;

-- name: MissingKRAEvents :many
SELECT p.id FROM payouts.payouts p
WHERE p.status = 'succeeded' AND p.tax_kes > 0
  AND NOT EXISTS (SELECT 1 FROM audit.kra_outbox k WHERE k.event_type = 'withholding_tax' AND k.payload->>'payout_id' = p.id::text);

-- name: LivePayoutTotals :one
SELECT coalesce(sum(net_kes), 0)::bigint AS net_kes, coalesce(sum(tax_kes), 0)::bigint AS tax_kes, count(*)::bigint AS payouts
FROM payouts.payouts WHERE status <> 'failed';

-- name: AccountBalance :one
-- credit-normal balance of one account
SELECT coalesce(sum(CASE e.direction WHEN 'C' THEN e.amount_kes ELSE -e.amount_kes END), 0)::bigint
FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id WHERE a.code = $1;

-- name: InsertReconciliationRun :exec
INSERT INTO payouts.reconciliation_runs (ran_at, ok, report) VALUES ($1, $2, $3);
