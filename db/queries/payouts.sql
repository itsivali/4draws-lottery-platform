-- name: InsertPayout :one
INSERT INTO payouts.payouts (user_id, kind, route, status, gross_kes, tax_kes, net_kes, msisdn, draw_result_id,
                             originator_conversation_id, idempotency_key, tax_code, tax_rate, tax_rounding, created_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(kind), 'b2c', sqlc.arg(status), sqlc.arg(gross_kes), sqlc.arg(tax_kes), sqlc.arg(net_kes),
        sqlc.arg(msisdn), sqlc.narg(draw_result_id), platform.uuid_v7()::text, sqlc.arg(idempotency_key), sqlc.arg(tax_code),
        (sqlc.arg(tax_rate)::text)::numeric, sqlc.arg(tax_rounding), sqlc.arg(at), sqlc.arg(at))
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
