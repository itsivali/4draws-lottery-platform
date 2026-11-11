# 0025. Alerts that reach people: more checks, escalation, email, and a page that explains

Status: accepted
Date: 2026-10-31

## Context
The alert watch (migration 00026, `internal/alerts`) knew six conditions: late draws, failed or missing
reconciliations, tax nearing its deadline, and a check that cannot run. It sent them to one webhook, or the log. Money
that needed a person (a top-up M-Pesa took but never confirmed, paybill money in suspense, a payout waiting for
approval or failed) and the platform's health (jobs River gave up on, Daraja throttling, an empty B2C float) were
visible only to someone who went looking on the right page at the right time. The owner asked on 2026-10-31 to
"expand the alerts system": operators must be told, in time and in the right channel, and the Alerts page must let
them see, filter and understand every alert.

## Decision
- **Eleven more kinds**, each owned by a check so it clears itself when the condition is gone: `topup.unconfirmed`,
  `topup.overdue_pending`, `topup.orphan_callbacks`, `paybill.suspense_open`, `paybill.unverified`,
  `payout.awaiting_approval`, `payout.failed`, `b2c.float_low`, `jobs.failing`, `daraja.throttled`,
  `kyc.queue_stale`, `closure.waiting`. One alert per thing (the payment, receipt, payout, job kind, user), a summary a
  person reads at a glance, details with the ids and amounts. Numbers are masked in alerts; the back office shows
  them. The checks read the tables that already record these things; nothing new is stored.
- **Severity escalates, never relaxes.** A warning raised again as a page (the same condition, older) becomes a page:
  the upsert in `RaiseAlert` raises the severity, records `escalated_at`/`escalated_from` in the details (kept across
  later refreshes of the details), and clears `notified_at` so the page is sent as an `escalated` event and then
  reminded hourly like any page. A page raised again as a warning stays a page until it resolves. Migration 00044
  relaxes the alert guard for exactly this one change (`warn` → `page` while open); lowering a severity is still
  refused.
- **Thresholds are configuration** with the owner's defaults, named in `docs/runbooks/alerts.md` and `render.yaml`
  (`TOPUP_UNCONFIRMED_WARN_AFTER` 30m / `_PAGE_AFTER` 4h, `TOPUP_PENDING_GRACE` 10m, `SUSPENSE_WARN_AFTER` 1h /
  `_PAGE_AFTER` 24h, `PAYOUT_APPROVAL_WARN_AFTER` 2h / `_PAGE_AFTER` 24h, `KYC_QUEUE_WARN_AFTER` 24h / `_PAGE_AFTER`
  72h, `CLOSURE_WAIT_WARN_AFTER` 168h, `JOB_RETRY_WARN_ATTEMPTS` 5, `DARAJA_THROTTLE_WINDOW` 15m /
  `DARAJA_THROTTLE_COUNT` 5, `B2C_FLOAT_FLOOR_KES` 0 = off). `config.AlertThresholds` mirrors `alerts.Thresholds`
  field for field and a test keeps the defaults identical. Things that *happened* (orphan callbacks, failed payouts,
  discarded jobs) are looked for over the last 24 hours, so such an alert clears a day after the last occurrence; the
  pages were sent and reminded meanwhile, and the System and Payouts pages keep the record.
- **Channels fan out.** `Email` sends each event to `ALERT_EMAILS` through the SMTP sender the platform already uses
  for staff email, with the subject `[4draws] PAGE kind: summary`, the details as `key: value` lines and the link
  `BACKOFFICE_URL/alerts?alert=<id>`. `Multi` sends to webhook and email; a channel that fails is logged and does not
  stop the others, and the event is retried at the next watch only when every channel failed (retrying after a
  partial failure would repeat the notice on the channel that worked). With no channel the log remains, as before.
  Reminders are for pages only, as before.
- **The API lists and explains.** `GET /v1/staff/alerts` takes `kind`, `severity`, `state` (`open` = not resolved,
  the default and what `?all=true` used to negate; `acknowledged`; `resolved`; `all`), `limit` (1–200) and `before`
  (a cursor on `first_seen_at|id`), and answers `{alerts, next_before, kinds}` where `kinds` is every kind the checks
  can raise with its open count. `GET /v1/staff/alerts/{id}` adds the owning check and a `history` built from the
  row (raised, escalated, resolved) and the audit log (`alert.acknowledged` with its note). `?all=true` keeps working
  for `alertWatch` and older `lotteryctl` builds.
- **The Alerts page** filters by state, kind (with counts) and severity, lists summary, kind·subject, state with who
  and when, first/last seen and occurrences, and opens each alert in a sheet: details as rows, a short "what to do"
  per kind with a link to the page that handles it (`/topups?topup=ID`, `/payments`, `/payouts`, `/system`, `/kyc`,
  `/closures`, `/draws`, `/tax`, `/finance`), the acknowledge action and the history. The banner for pages and
  freezes on every page stays. `lotteryctl alerts list` takes `-kind` and `-state`.

## Consequences
- An operator who reads only email, or only Slack, sees the same alert; a stuck top-up or a suspense item is noticed
  within the hour instead of at the next reconciliation. A warning nobody acted on becomes a page on its own.
- More alerts means more noise if thresholds are wrong; they are environment variables so the owner tunes them
  without a release, and the runbook names each with its default.
- `b2c.float_low` is off until the owner sets a floor: the ledger only sees the float once funding is recorded.
- Escalation changes a column the guard used to freeze; the guard now permits the one direction that makes sense,
  and the integration test that tried to lower a severity still fails as before.
