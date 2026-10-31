-- name: RaiseAlert :one
-- one open alert per (kind, subject): raising it again counts the occurrence and refreshes what it says. A warning
-- raised again as a page escalates (ADR 0025): it becomes a page, remembers when in its details, and is sent again
-- (notified_at cleared). A page raised again as a warning stays a page.
INSERT INTO audit.alerts (kind, subject, severity, freezes, summary, details, first_seen_at, last_seen_at)
VALUES (sqlc.arg(kind), sqlc.arg(subject), sqlc.arg(severity), sqlc.narg(freezes), sqlc.arg(summary), sqlc.arg(details), sqlc.arg(at), sqlc.arg(at))
ON CONFLICT (kind, subject) WHERE resolved_at IS NULL
DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at, occurrences = audit.alerts.occurrences + 1, summary = EXCLUDED.summary,
              severity = CASE WHEN audit.alerts.severity = 'warn' AND EXCLUDED.severity = 'page' THEN 'page' ELSE audit.alerts.severity END,
              details = EXCLUDED.details || CASE
                WHEN audit.alerts.severity = 'warn' AND EXCLUDED.severity = 'page'
                  THEN jsonb_build_object('escalated_at', EXCLUDED.last_seen_at, 'escalated_from', 'warn')
                WHEN audit.alerts.details ? 'escalated_at'
                  THEN jsonb_build_object('escalated_at', audit.alerts.details->'escalated_at', 'escalated_from', audit.alerts.details->'escalated_from')
                ELSE '{}'::jsonb END,
              notified_at = CASE WHEN audit.alerts.severity = 'warn' AND EXCLUDED.severity = 'page' THEN NULL ELSE audit.alerts.notified_at END
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
-- newest first, by kind, severity and state (open: not resolved, acknowledged or not; acknowledged: acknowledged and
-- not resolved; resolved; anything else: all), continued from before (first_seen_at, id)
SELECT * FROM audit.alerts
WHERE (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
  AND (sqlc.narg(severity)::text IS NULL OR severity = sqlc.narg(severity))
  AND (CASE sqlc.arg(state)::text
         WHEN 'open' THEN resolved_at IS NULL
         WHEN 'acknowledged' THEN resolved_at IS NULL AND acknowledged_at IS NOT NULL
         WHEN 'resolved' THEN resolved_at IS NOT NULL
         ELSE true END)
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (first_seen_at, id) < (sqlc.narg(before_at), sqlc.narg(before_id)::uuid))
ORDER BY first_seen_at DESC, id DESC LIMIT sqlc.arg(max_rows);

-- name: OpenAlertCounts :many
-- how many alerts of each kind are not resolved
SELECT kind, count(*)::bigint AS open FROM audit.alerts WHERE resolved_at IS NULL GROUP BY kind ORDER BY kind;

-- name: GetAlert :one
SELECT * FROM audit.alerts WHERE id = $1;

-- name: AlertAuditEvents :many
-- what people did about the alert, as the audit log recorded it
SELECT id, at, actor_type, actor_id, action, details FROM audit.audit_log
WHERE entity_type = 'alert' AND entity_id = $1 ORDER BY at, id;

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

-- name: UnconfirmedTopUps :many
-- top-ups M-Pesa may have taken that were never confirmed (status unconfirmed), oldest first
SELECT id, amount_requested_kes, msisdn, failure_reason, created_at, updated_at
FROM payments.payments WHERE status = 'unconfirmed' ORDER BY created_at, id;

-- name: PendingTopUpsStartedBefore :many
-- top-ups still pending (a stuck job: the verify job settles every pending top-up at expiry) that started before the cutoff
SELECT id, status, amount_requested_kes, msisdn, created_at
FROM payments.payments WHERE status IN ('initiated','stk_sent','callback_received') AND created_at < sqlc.arg(before) ORDER BY created_at, id;

-- name: OrphanCallbacksSince :many
-- STK callbacks that matched no payment, per checkout id, since the cutoff
SELECT checkout_request_id, count(*)::bigint AS callbacks, min(received_at)::timestamptz AS first_at, max(received_at)::timestamptz AS last_at
FROM payments.stk_orphan_callbacks WHERE received_at >= sqlc.arg(since) GROUP BY checkout_request_id ORDER BY 3;

-- name: OpenSuspense :many
-- paybill money parked in suspense that no operator has settled, oldest first
SELECT id, mpesa_receipt, amount_kes, reason, created_at FROM payments.c2b_unmatched WHERE status = 'open' ORDER BY created_at, id;

-- name: UnverifiedPaybill :many
-- paybill confirmations Daraja's status query could not verify, oldest first
SELECT trans_id, amount_kes, bill_ref, outcome_reason, created_at FROM payments.c2b_confirmations WHERE status = 'unverified' ORDER BY created_at, id;

-- name: KYCReviewQueue :one
-- accounts waiting for a staff decision and when the oldest of their latest submissions arrived (as kyc.ReviewQueue;
-- oldest is meaningless when nobody waits)
SELECT count(*)::bigint AS waiting, coalesce(min(c.created_at), now())::timestamptz AS oldest
FROM identity.users u
JOIN LATERAL (SELECT k.created_at FROM kyc.kyc_checks k WHERE k.user_id = u.id ORDER BY k.created_at DESC, k.id DESC LIMIT 1) c ON true
WHERE u.kyc_status = 'manual_review';

-- name: PayoutsAwaitingApproval :many
-- payouts waiting for an operator: held (a withdrawal above the automatic limit) or manual (an outcome Daraja could not confirm)
SELECT id, kind, status, net_kes, reason, created_at, updated_at FROM payouts.payouts WHERE status IN ('held','manual') ORDER BY updated_at, id;

-- name: FailedPayoutsSince :many
-- payouts that failed (money back in the wallet or the prize pool) since the cutoff
SELECT id, kind, net_kes, reason, updated_at FROM payouts.payouts WHERE status = 'failed' AND updated_at >= sqlc.arg(since) ORDER BY updated_at, id;

-- name: B2CFloatBalance :one
-- the B2C float as the ledger sees it (an asset: debits add, credits take), in whole KES
SELECT coalesce(sum(CASE e.direction WHEN 'D' THEN e.amount_kes ELSE -e.amount_kes END), 0)::bigint
FROM ledger.ledger_entries e JOIN ledger.ledger_accounts a ON a.id = e.account_id WHERE a.code = 'B2C_FLOAT';

-- name: OpenClosureRequestsBefore :many
-- account closures requested before the cutoff that nobody has carried out
SELECT user_id, requested_at FROM identity.closure_requests WHERE closed_at IS NULL AND requested_at < sqlc.arg(before) ORDER BY requested_at;

-- name: OpenReceiptClaims :many
-- receipts players gave for uncredited top-ups (ADR 0024) that M-Pesa has not confirmed, oldest first
SELECT id, amount_requested_kes, msisdn, claimed_receipt, claim_status, claim_note, claimed_at
FROM payments.payments
WHERE claim_status IN ('claimed','checking','unverified') AND claimed_at IS NOT NULL
ORDER BY claimed_at, id;
