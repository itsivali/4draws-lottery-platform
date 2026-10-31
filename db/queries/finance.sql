-- name: Flows :many
-- every ledger movement in [from_at, to_at), summed per Nairobi day, journal kind, account and side
SELECT (j.created_at AT TIME ZONE 'Africa/Nairobi')::date AS day, j.kind, a.code AS account, e.direction,
       sum(e.amount_kes)::bigint AS amount_kes
FROM ledger.ledger_entries e
JOIN ledger.ledger_journals j ON j.id = e.journal_id
JOIN ledger.ledger_accounts a ON a.id = e.account_id
WHERE j.created_at >= sqlc.arg(from_at) AND j.created_at < sqlc.arg(to_at)
GROUP BY 1, 2, 3, 4
ORDER BY 1, 2, 3, 4;

-- name: Balances :many
-- each account's balance now, as debits less credits
SELECT a.code, a.type, coalesce(sum(CASE e.direction WHEN 'D' THEN e.amount_kes ELSE -e.amount_kes END), 0)::bigint AS debit_balance
FROM ledger.ledger_accounts a
LEFT JOIN ledger.ledger_entries e ON e.account_id = a.id
GROUP BY a.code, a.type
ORDER BY a.code;

-- name: Activity :one
-- players who signed up, and plays and the players who made them, in [from_at, to_at); play dates are Nairobi dates
SELECT (SELECT count(*) FROM identity.users u WHERE u.created_at >= sqlc.arg(from_at) AND u.created_at < sqlc.arg(to_at))::bigint AS signups,
       (SELECT count(*) FROM tickets.tickets t WHERE t.status = 'paid' AND t.play_date BETWEEN sqlc.arg(from_day)::date AND sqlc.arg(to_day)::date)::bigint AS plays,
       (SELECT count(DISTINCT t.user_id) FROM tickets.tickets t WHERE t.status = 'paid' AND t.play_date BETWEEN sqlc.arg(from_day)::date AND sqlc.arg(to_day)::date)::bigint AS players;

-- name: PlaysByPeriod :many
-- paid plays and the distinct players who made them, per period (unit: day, week or month; date_trunc's weeks start on
-- Monday) of the Nairobi play date, in [from_day, to_day]
SELECT date_trunc(sqlc.arg(unit)::text, t.play_date::timestamp)::date AS bucket,
       count(*)::bigint AS plays, count(DISTINCT t.user_id)::bigint AS players
FROM tickets.tickets t
WHERE t.status = 'paid' AND t.play_date BETWEEN sqlc.arg(from_day)::date AND sqlc.arg(to_day)::date
GROUP BY 1
ORDER BY 1;

-- name: SignupsByDay :many
-- accounts created per Nairobi day in [from_at, to_at)
SELECT (u.created_at AT TIME ZONE 'Africa/Nairobi')::date AS day, count(*)::bigint AS signups
FROM identity.users u
WHERE u.created_at >= sqlc.arg(from_at) AND u.created_at < sqlc.arg(to_at)
GROUP BY 1
ORDER BY 1;

-- name: JournalKinds :many
-- every journal kind posted in [from_at, to_at): how many journals, and what they debited and credited in all
SELECT j.kind, count(DISTINCT j.id)::bigint AS journals,
       coalesce(sum(e.amount_kes) FILTER (WHERE e.direction = 'D'), 0)::bigint AS debit_kes,
       coalesce(sum(e.amount_kes) FILTER (WHERE e.direction = 'C'), 0)::bigint AS credit_kes
FROM ledger.ledger_journals j
JOIN ledger.ledger_entries e ON e.journal_id = j.id
WHERE j.created_at >= sqlc.arg(from_at) AND j.created_at < sqlc.arg(to_at)
GROUP BY j.kind
ORDER BY j.kind;

-- name: TrialBalance :many
-- every account's debits and credits from the beginning of time to before as_at (the end of the range's last day)
SELECT a.code, a.name, a.type,
       coalesce(sum(e.amount_kes) FILTER (WHERE e.direction = 'D' AND j.created_at < sqlc.arg(as_at)), 0)::bigint AS debits_kes,
       coalesce(sum(e.amount_kes) FILTER (WHERE e.direction = 'C' AND j.created_at < sqlc.arg(as_at)), 0)::bigint AS credits_kes
FROM ledger.ledger_accounts a
LEFT JOIN ledger.ledger_entries e ON e.account_id = a.id
LEFT JOIN ledger.ledger_journals j ON j.id = e.journal_id
GROUP BY a.code, a.name, a.type
ORDER BY a.code;

-- name: TopUpFunnel :many
-- top-ups started per period (of their Nairobi day) in [from_at, to_at), by how they ended; the median seconds from
-- the prompt to the credit is -1 when nothing was credited in the period
SELECT date_trunc(sqlc.arg(unit)::text, (p.created_at AT TIME ZONE 'Africa/Nairobi')::date::timestamp)::date AS bucket,
       count(*)::bigint AS started, coalesce(sum(p.amount_requested_kes), 0)::bigint AS started_kes,
       count(*) FILTER (WHERE p.status IN ('credited', 'reversed'))::bigint AS credited,
       coalesce(sum(p.amount_confirmed_kes) FILTER (WHERE p.status IN ('credited', 'reversed')), 0)::bigint AS credited_kes,
       count(*) FILTER (WHERE p.status = 'failed')::bigint AS failed,
       coalesce(sum(p.amount_requested_kes) FILTER (WHERE p.status = 'failed'), 0)::bigint AS failed_kes,
       count(*) FILTER (WHERE p.status = 'cancelled')::bigint AS cancelled,
       coalesce(sum(p.amount_requested_kes) FILTER (WHERE p.status = 'cancelled'), 0)::bigint AS cancelled_kes,
       count(*) FILTER (WHERE p.status = 'expired')::bigint AS expired,
       coalesce(sum(p.amount_requested_kes) FILTER (WHERE p.status = 'expired'), 0)::bigint AS expired_kes,
       count(*) FILTER (WHERE p.status = 'unconfirmed')::bigint AS unconfirmed,
       coalesce(sum(p.amount_requested_kes) FILTER (WHERE p.status = 'unconfirmed'), 0)::bigint AS unconfirmed_kes,
       count(*) FILTER (WHERE p.status = 'reversed')::bigint AS reversed,
       coalesce(sum(p.amount_confirmed_kes) FILTER (WHERE p.status = 'reversed'), 0)::bigint AS reversed_kes,
       count(*) FILTER (WHERE p.status IN ('initiated', 'stk_sent', 'callback_received', 'verified'))::bigint AS pending_now,
       coalesce(sum(p.amount_requested_kes) FILTER (WHERE p.status IN ('initiated', 'stk_sent', 'callback_received', 'verified')), 0)::bigint AS pending_now_kes,
       coalesce(round(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM p.updated_at - p.created_at))
                      FILTER (WHERE p.status = 'credited'))::bigint, -1)::bigint AS median_seconds_to_credit
FROM payments.payments p
WHERE p.created_at >= sqlc.arg(from_at) AND p.created_at < sqlc.arg(to_at)
GROUP BY 1
ORDER BY 1;

-- name: PayoutFunnel :many
-- payouts requested per period (of their Nairobi day) in [from_at, to_at), by where they stand now
SELECT date_trunc(sqlc.arg(unit)::text, (p.created_at AT TIME ZONE 'Africa/Nairobi')::date::timestamp)::date AS bucket,
       count(*)::bigint AS requested, coalesce(sum(p.gross_kes), 0)::bigint AS requested_kes,
       count(*) FILTER (WHERE p.status = 'succeeded')::bigint AS paid,
       coalesce(sum(p.net_kes) FILTER (WHERE p.status = 'succeeded'), 0)::bigint AS paid_kes,
       count(*) FILTER (WHERE p.status = 'failed')::bigint AS failed,
       coalesce(sum(p.gross_kes) FILTER (WHERE p.status = 'failed'), 0)::bigint AS failed_kes,
       count(*) FILTER (WHERE p.status = 'held')::bigint AS held,
       coalesce(sum(p.net_kes) FILTER (WHERE p.status = 'held'), 0)::bigint AS held_kes,
       count(*) FILTER (WHERE p.status = 'manual')::bigint AS manual,
       coalesce(sum(p.net_kes) FILTER (WHERE p.status = 'manual'), 0)::bigint AS manual_kes,
       coalesce(sum(p.tax_kes) FILTER (WHERE p.status = 'succeeded'), 0)::bigint AS tax_withheld_kes
FROM payouts.payouts p
WHERE p.created_at >= sqlc.arg(from_at) AND p.created_at < sqlc.arg(to_at)
GROUP BY 1
ORDER BY 1;

-- name: TaxPosition :one
-- withholding tax not yet remitted: batches prepared and not yet remitted, withholding events not yet in a batch, and
-- the oldest withholding event not covered by a remitted batch (the zero time when there is none)
SELECT (SELECT count(*) FROM audit.kra_batches b WHERE b.prn IS NULL)::bigint AS awaiting_batches,
       (SELECT coalesce(sum(b.tax_kes), 0) FROM audit.kra_batches b WHERE b.prn IS NULL)::bigint AS awaiting_kes,
       (SELECT count(*) FROM audit.kra_outbox o WHERE o.event_type = 'withholding_tax' AND o.batch_id IS NULL)::bigint AS unbatched_events,
       (SELECT coalesce(sum((o.payload->>'tax_kes')::bigint), 0) FROM audit.kra_outbox o WHERE o.event_type = 'withholding_tax' AND o.batch_id IS NULL)::bigint AS unbatched_kes,
       coalesce((SELECT min(o.created_at) FROM audit.kra_outbox o LEFT JOIN audit.kra_batches b ON b.id = o.batch_id
                 WHERE o.event_type = 'withholding_tax' AND b.prn IS NULL), '0001-01-01 00:00:00+00'::timestamptz)::timestamptz AS oldest_unremitted_at;

-- name: CountLedgerEntries :one
-- how many ledger entries the export of [from_at, to_at) would hold
SELECT count(*)::bigint FROM ledger.ledger_entries e
JOIN ledger.ledger_journals j ON j.id = e.journal_id
WHERE j.created_at >= sqlc.arg(from_at) AND j.created_at < sqlc.arg(to_at);
