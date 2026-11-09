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
-- Oldest first (the operator's queue); after (a payout id) continues a page from the payout after it.
SELECT p.* FROM payouts.payouts p
WHERE p.status = sqlc.arg(status)
  AND (sqlc.narg(after)::uuid IS NULL
       OR (p.created_at, p.id) > (SELECT a.created_at, a.id FROM payouts.payouts a WHERE a.id = sqlc.narg(after)::uuid))
ORDER BY p.created_at, p.id
LIMIT sqlc.arg(max_rows);

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
-- a reason the payout was held for (ADR 0030) no longer applies once a person released it; the audit log keeps it
UPDATE payouts.payouts SET status = 'pending', reason = NULL, approved_by = sqlc.arg(approved_by), updated_at = sqlc.arg(at)
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

-- name: UnbatchedKRAEvents :many
-- withholding events in no batch yet (events exported before batches existed are batched too: they were never remitted)
SELECT id, event_type, payload, created_at FROM audit.kra_outbox WHERE batch_id IS NULL AND event_type = 'withholding_tax'
ORDER BY id LIMIT sqlc.arg(max_rows) FOR UPDATE;

-- name: NextKRABatchNo :one
-- called under the batch advisory lock, so numbers have no gaps
SELECT (coalesce(max(batch_no), 0) + 1)::bigint FROM audit.kra_batches;

-- name: InsertKRABatch :one
INSERT INTO audit.kra_batches (batch_no, events, first_event_at, last_event_at, gross_kes, tax_kes, file_sha256, prepared_by, prepared_at)
VALUES (sqlc.arg(batch_no), sqlc.arg(events), sqlc.arg(first_event_at), sqlc.arg(last_event_at), sqlc.arg(gross_kes), sqlc.arg(tax_kes),
        sqlc.arg(file_sha256), sqlc.arg(prepared_by), sqlc.arg(at))
RETURNING id;

-- name: AssignKRABatch :exec
UPDATE audit.kra_outbox SET batch_id = sqlc.arg(batch_id), exported_at = coalesce(exported_at, sqlc.arg(at)::timestamptz)
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: KRABatchByNo :one
SELECT * FROM audit.kra_batches WHERE batch_no = $1;

-- name: LockKRABatchByNo :one
SELECT * FROM audit.kra_batches WHERE batch_no = $1 FOR UPDATE;

-- name: KRABatchEvents :many
SELECT id, event_type, payload, created_at FROM audit.kra_outbox WHERE batch_id = $1 ORDER BY id;

-- name: KRABatches :many
SELECT * FROM audit.kra_batches ORDER BY batch_no DESC LIMIT sqlc.arg(max_rows);

-- name: PRNUsed :one
SELECT EXISTS (SELECT 1 FROM audit.kra_batches WHERE prn = $1)::bool;

-- name: RecordKRARemittance :exec
UPDATE audit.kra_batches SET prn = sqlc.arg(prn), remitted_kes = sqlc.arg(remitted_kes), remitted_by = sqlc.arg(remitted_by),
  remitted_at = sqlc.arg(at)
WHERE id = sqlc.arg(id);

-- name: RemittedTax :one
SELECT coalesce(sum(remitted_kes), 0)::bigint FROM audit.kra_batches;

-- name: KRABatchTotalsMismatch :many
-- a batch whose control totals are not those of its events
SELECT b.batch_no::text FROM audit.kra_batches b
CROSS JOIN LATERAL (SELECT count(*) AS n, coalesce(sum((o.payload->>'tax_kes')::bigint), 0)::bigint AS tax,
                           coalesce(sum((o.payload->>'gross_kes')::bigint), 0)::bigint AS gross
                    FROM audit.kra_outbox o WHERE o.batch_id = b.id) e
WHERE e.n <> b.events OR e.tax <> b.tax_kes OR e.gross <> b.gross_kes;

-- name: RemittanceJournalMismatch :many
-- a remitted batch without exactly one remittance journal of its tax, or a remittance journal for an unremitted batch
SELECT b.batch_no::text FROM audit.kra_batches b
WHERE (SELECT count(*) FROM ledger.ledger_journals j WHERE j.kind = 'tax_remittance' AND j.reference_type = 'kra_batch' AND j.reference_id = b.id)
      <> CASE WHEN b.prn IS NULL THEN 0 ELSE 1 END
   OR (b.prn IS NOT NULL AND coalesce((SELECT sum(e.amount_kes) FROM ledger.ledger_journals j
          JOIN ledger.ledger_entries e ON e.journal_id = j.id JOIN ledger.ledger_accounts a ON a.id = e.account_id
          WHERE j.kind = 'tax_remittance' AND j.reference_type = 'kra_batch' AND j.reference_id = b.id
            AND a.code = 'TAX_PAYABLE' AND e.direction = 'D'), 0) <> b.remitted_kes);

-- name: KRAEventMismatch :many
-- a withholding event that is not its payout's: another tax or gross, or a payout that did not succeed
SELECT o.id::text FROM audit.kra_outbox o
LEFT JOIN payouts.payouts p ON p.id::text = o.payload->>'payout_id'
WHERE o.event_type = 'withholding_tax'
  AND (p.id IS NULL OR p.status <> 'succeeded' OR (o.payload->>'tax_kes')::bigint <> p.tax_kes OR (o.payload->>'gross_kes')::bigint <> p.gross_kes);

-- name: PayoutsFrozen :one
-- an open alert that freezes payouts (closed only by a person: lotteryctl alerts ack)
SELECT EXISTS (SELECT 1 FROM audit.alerts WHERE freezes = 'payouts' AND resolved_at IS NULL)::bool;

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

-- name: UnsettledPayouts :one
-- Payouts of the player not yet paid or failed (an account cannot be closed while money may still move).
SELECT count(*)::int FROM payouts.payouts
WHERE user_id = sqlc.arg(user_id) AND status NOT IN ('succeeded','failed');

-- name: PrizeInterest :one
-- What a player may still win or be owed from draws: paid tickets on a day of a draw still taking entries, entries in
-- a draw not yet paid or voided, and won prizes without a successful payout. An account is not closed while any remain.
SELECT
  (SELECT count(*) FROM tickets.tickets t JOIN draws.draws d
     ON d.status = 'open' AND t.play_date BETWEEN d.period_start AND d.period_end
   WHERE t.user_id = sqlc.arg(user_id) AND t.status = 'paid')::int AS open_draw_tickets,
  (SELECT count(*) FROM draws.draw_entries e JOIN draws.draws d ON d.id = e.draw_id
   WHERE e.user_id = sqlc.arg(user_id) AND d.status NOT IN ('paid','voided'))::int AS undecided_entries,
  (SELECT count(*) FROM draws.draw_results r JOIN draws.draw_entries e ON e.id = r.entry_id
   WHERE e.user_id = sqlc.arg(user_id)
     AND NOT EXISTS (SELECT 1 FROM payouts.payouts p WHERE p.draw_result_id = r.id AND p.status = 'succeeded'))::int AS unpaid_prizes;
