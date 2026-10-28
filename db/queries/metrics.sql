-- App metrics for the back office (ADR 0019): how the player apps are used, per platform (web, android, ios: the
-- platform a session was opened from, kept on the session; older sessions have it on their device) and Nairobi day. Everything is read from the platform's own
-- tables; nothing is sampled or sent anywhere.

-- name: Signups :many
-- players who signed up in [from_at, to_at), by the platform of their first session
SELECT (u.created_at AT TIME ZONE 'Africa/Nairobi')::date AS day, coalesce(f.platform, 'unknown')::text AS platform, count(*)::bigint AS n
FROM identity.users u
LEFT JOIN LATERAL (SELECT coalesce(s.platform, d.platform) AS platform FROM identity.sessions s LEFT JOIN identity.devices d ON d.id = s.device_id
                   WHERE s.user_id = u.id ORDER BY s.created_at, s.id LIMIT 1) f ON true
WHERE u.created_at >= sqlc.arg(from_at) AND u.created_at < sqlc.arg(to_at)
GROUP BY 1, 2;

-- name: Sessions :many
-- sign-ins (sessions opened) in [from_at, to_at), by platform
SELECT (s.created_at AT TIME ZONE 'Africa/Nairobi')::date AS day, coalesce(s.platform, d.platform, 'unknown')::text AS platform, count(*)::bigint AS n
FROM identity.sessions s
LEFT JOIN identity.devices d ON d.id = s.device_id
WHERE s.created_at >= sqlc.arg(from_at) AND s.created_at < sqlc.arg(to_at)
GROUP BY 1, 2;

-- name: ActivePlayers :many
-- players seen on a day (a token was issued to them: sign-in or refresh), once per platform they used that day
SELECT day, platform, count(DISTINCT user_id)::bigint AS n
FROM (SELECT (r.issued_at AT TIME ZONE 'Africa/Nairobi')::date AS day, coalesce(s.platform, d.platform, 'unknown')::text AS platform, s.user_id
      FROM identity.refresh_tokens r
      JOIN identity.sessions s ON s.id = r.session_id
      LEFT JOIN identity.devices d ON d.id = s.device_id
      WHERE r.issued_at >= sqlc.arg(from_at) AND r.issued_at < sqlc.arg(to_at)) x
GROUP BY 1, 2;

-- name: Money :many
-- plays, top-ups and withdrawals per day, attributed to the platform the player used that day (the device of their
-- latest token that day), else their latest session, else unknown. Money comes from the wallet's own transactions.
WITH presence AS (
  SELECT DISTINCT ON (s.user_id, (r.issued_at AT TIME ZONE 'Africa/Nairobi')::date)
         s.user_id, (r.issued_at AT TIME ZONE 'Africa/Nairobi')::date AS day, coalesce(s.platform, d.platform, 'unknown')::text AS platform
  FROM identity.refresh_tokens r
  JOIN identity.sessions s ON s.id = r.session_id
  LEFT JOIN identity.devices d ON d.id = s.device_id
  WHERE r.issued_at >= sqlc.arg(from_at) AND r.issued_at < sqlc.arg(to_at)
  ORDER BY s.user_id, (r.issued_at AT TIME ZONE 'Africa/Nairobi')::date, r.issued_at DESC
), last_device AS (
  SELECT DISTINCT ON (s.user_id) s.user_id, coalesce(s.platform, d.platform) AS platform
  FROM identity.sessions s LEFT JOIN identity.devices d ON d.id = s.device_id ORDER BY s.user_id, s.created_at DESC, s.id DESC
), events AS (
  SELECT t.user_id, t.play_date AS day, 'play'::text AS kind, t.price_kes AS kes
  FROM tickets.tickets t WHERE t.status = 'paid' AND t.play_date BETWEEN sqlc.arg(from_day)::date AND sqlc.arg(to_day)::date
  UNION ALL
  SELECT w.user_id, (w.created_at AT TIME ZONE 'Africa/Nairobi')::date, CASE w.kind WHEN 'deposit' THEN 'top_up' ELSE 'withdrawal' END, abs(w.amount_kes)
  FROM wallet.wallet_transactions w
  WHERE w.kind IN ('deposit', 'withdrawal') AND w.created_at >= sqlc.arg(from_at) AND w.created_at < sqlc.arg(to_at)
)
SELECT e.day, coalesce(pr.platform, ld.platform, 'unknown')::text AS platform, e.kind, count(*)::bigint AS n, coalesce(sum(e.kes), 0)::bigint AS kes
FROM events e
LEFT JOIN presence pr ON pr.user_id = e.user_id AND pr.day = e.day
LEFT JOIN last_device ld ON ld.user_id = e.user_id
GROUP BY 1, 2, 3
ORDER BY 1, 2, 3;
