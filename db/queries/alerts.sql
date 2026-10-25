-- name: RaiseAlert :one
-- one open alert per (kind, subject): raising it again counts the occurrence and refreshes what it says
INSERT INTO audit.alerts (kind, subject, severity, freezes, summary, details, first_seen_at, last_seen_at)
VALUES (sqlc.arg(kind), sqlc.arg(subject), sqlc.arg(severity), sqlc.narg(freezes), sqlc.arg(summary), sqlc.arg(details), sqlc.arg(at), sqlc.arg(at))
ON CONFLICT (kind, subject) WHERE resolved_at IS NULL
DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at, occurrences = audit.alerts.occurrences + 1, summary = EXCLUDED.summary,
              details = EXCLUDED.details
RETURNING id, (xmax = 0)::bool AS created;

-- name: OpenAlertsOfKind :many
SELECT id, subject, freezes FROM audit.alerts WHERE kind = $1 AND resolved_at IS NULL;

-- name: ResolveAlert :exec
-- the condition is gone; an alert that freezes payouts is never closed this way
UPDATE audit.alerts SET resolved_by = sqlc.arg(resolved_by), resolved_at = sqlc.arg(at)
WHERE id = sqlc.arg(id) AND resolved_at IS NULL AND freezes IS NULL;

-- name: LockAlert :one
SELECT * FROM audit.alerts WHERE id = $1 FOR UPDATE;

-- name: AcknowledgeAlert :exec
UPDATE audit.alerts SET acknowledged_by = sqlc.arg(operator), acknowledged_at = sqlc.arg(at), ack_note = sqlc.arg(note),
  resolved_by = CASE WHEN sqlc.arg(resolve)::bool THEN sqlc.arg(operator) ELSE resolved_by END,
  resolved_at = CASE WHEN sqlc.arg(resolve)::bool THEN sqlc.arg(at) ELSE resolved_at END
WHERE id = sqlc.arg(id);

-- name: AlertsToNotify :many
-- new alerts, unacknowledged pages due a reminder, and resolutions of alerts that were sent
SELECT * FROM audit.alerts
WHERE (resolved_at IS NULL AND notified_at IS NULL)
   OR (resolved_at IS NULL AND severity = 'page' AND acknowledged_at IS NULL AND notified_at < sqlc.arg(remind_before))
   OR (resolved_at IS NOT NULL AND notified_at IS NOT NULL AND resolution_notified_at IS NULL)
ORDER BY first_seen_at
LIMIT 50;

-- name: MarkNotified :exec
UPDATE audit.alerts SET notified_at = sqlc.arg(at), notifications = notifications + 1 WHERE id = sqlc.arg(id);

-- name: MarkResolutionNotified :exec
UPDATE audit.alerts SET resolution_notified_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: ListAlerts :many
SELECT * FROM audit.alerts WHERE resolved_at IS NULL OR sqlc.arg(include_resolved)::bool
ORDER BY first_seen_at DESC LIMIT sqlc.arg(max_rows);

-- name: LatestReconciliations :many
-- the last run of each daily reconciliation that has run at least once
(SELECT 'payments'::text AS source, id, ran_at, ok, report FROM payments.reconciliation_runs ORDER BY ran_at DESC LIMIT 1)
UNION ALL
(SELECT 'game'::text, id, ran_at, ok, report FROM game.reconciliation_runs ORDER BY ran_at DESC LIMIT 1)
UNION ALL
(SELECT 'draws'::text, id, ran_at, ok, report FROM draws.reconciliation_runs ORDER BY ran_at DESC LIMIT 1)
UNION ALL
(SELECT 'payouts'::text, id, ran_at, ok, report FROM payouts.reconciliation_runs ORDER BY ran_at DESC LIMIT 1);

-- name: UnremittedWithholding :many
-- withholding tax events not yet in a remitted batch, oldest first
SELECT o.created_at, (o.payload->>'tax_kes')::bigint AS tax_kes, b.batch_no
FROM audit.kra_outbox o LEFT JOIN audit.kra_batches b ON b.id = o.batch_id
WHERE o.event_type = 'withholding_tax' AND b.prn IS NULL
ORDER BY o.created_at, o.id;
