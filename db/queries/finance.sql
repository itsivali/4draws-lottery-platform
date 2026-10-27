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
