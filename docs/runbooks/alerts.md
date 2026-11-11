# Runbook: alerts

Every 5 minutes the worker's `alerts.watch` runs the checks below. Each condition becomes **one** alert in
`audit.alerts` however often it fires. New alerts go to every configured channel: `ALERT_WEBHOOK_URL` (a
Slack-compatible incoming webhook that gets JSON with a `text` field) and `ALERT_EMAILS` (plain-text email through the
SMTP server, with a link that opens the alert in the back office). Pages are sent again every hour until someone
acknowledges them; warnings are sent once. A warning that waits past its page threshold **escalates**: it becomes a
page, is sent again, and its details record `escalated_at` (ADR 0025). When an alert's condition clears, it is
resolved and the resolution is sent too. Without any channel, alerts are logged (`ERROR alert`) and kept in the
database; the back office's Alerts page shows them either way.

## Every kind of alert

Thresholds are environment variables (Go durations such as `30m`, `4h`, `168h`); the defaults are in brackets.

| Alert | Severity | Raised when | Clears | What to do |
|---|---|---|---|---|
| `topup.unconfirmed` | warn → page | a top-up M-Pesa may have taken is `unconfirmed` for `TOPUP_UNCONFIRMED_WARN_AFTER` [30m] / `TOPUP_UNCONFIRMED_PAGE_AFTER` [4h] after the player started it | when an operator credits or fails it | Back office → Top-ups (the link opens it): find the receipt on the M-Pesa statement, credit it from the receipt or fail it ([payments](payments-operations.md)) |
| `topup.overdue_pending` | page | a top-up is still `initiated`/`stk_sent`/`callback_received` `TOPUP_PENDING_GRACE` [10m] past its 15-minute expiry | when it settles | The verify job did not run: check the background jobs on the System page; the top-up settles itself once the worker is back |
| `topup.receipt_claim` | warn → page | a receipt a player gave for an uncredited top-up (ADR 0024) that M-Pesa did not confirm (warn; page after `TOPUP_UNCONFIRMED_PAGE_AFTER`), or has not answered about after 15 min (warn) | when the top-up is credited or the claim rejected | Open the top-up: look for the receipt on the M-Pesa statement, then credit from it or close it as not on the statement |
| `topup.orphan_callbacks` | warn | STK callbacks of the last 24 h matched no top-up (one alert per checkout id) | 24 h after the last such callback | A push lost before it was saved, or a forgery: compare with the statement; nothing was credited |
| `paybill.suspense_open` | warn → page | paybill money is in suspense for `SUSPENSE_WARN_AFTER` [1h] / `SUSPENSE_PAGE_AFTER` [24h] | when credited or refunded | Back office → Payments → suspense: credit the right player or refund it |
| `paybill.unverified` | warn | a paybill confirmation is `unverified` (Daraja could not confirm it) | when rechecked or settled | Recheck it; or settle it from the statement |
| `payout.awaiting_approval` | warn → page | a payout is `held` or `manual` for `PAYOUT_APPROVAL_WARN_AFTER` [2h] / `PAYOUT_APPROVAL_PAGE_AFTER` [24h] | when decided | Back office → Payouts: approve or reject a held withdrawal; settle a manual one from the statement ([payouts](payouts.md)) |
| `payout.failed` | page | a payout failed in the last 24 h | 24 h after the failure | The money is back in the wallet or the pool: tell the player; retry a prize once the cause is fixed |
| `b2c.float_low` | warn | the ledger's B2C float is below `B2C_FLOAT_FLOOR_KES` [0 = off] | when topped up | Top the float up with Safaricom before payouts stall |
| `jobs.failing` | page / warn | a River job kind had a job **discarded** in the last 24 h (page), or one still retrying at attempt ≥ `JOB_RETRY_WARN_ATTEMPTS` [5] (warn); one alert per job kind | when the jobs are retried, done or cleaned up | System page: read the last error; a discarded job did not do its work and may need re-queuing by engineering |
| `daraja.throttled` | warn | ≥ `DARAJA_THROTTLE_COUNT` [5] errors saying "throttled" on the STK jobs in the last `DARAJA_THROTTLE_WINDOW` [15m] | when the window is quiet | Top-ups confirm late until it eases; if it persists, ask Safaricom about the rate limit |
| `kyc.queue_stale` | warn → page | the oldest identity submission awaiting a decision is `KYC_QUEUE_WARN_AFTER` [24h] / `KYC_QUEUE_PAGE_AFTER` [72h] old | when the queue is younger | Back office → Identity review ([kyc-review](kyc-review.md)) |
| `closure.waiting` | warn | an account closure request is open for `CLOSURE_WAIT_WARN_AFTER` [168h] | when carried out | Back office → Closures: refund what is left and close it |
| `draw.overdue` | page | a draw's next step is late (`lotteryctl draws overdue`; see [draws](draws.md)) | when the draw moves on | Sign, approve or void it |
| `reconciliation.failed` | page, **freezes payouts** | the latest payments, game, draws or payouts reconciliation found mismatches | only when a person acknowledges it | See below |
| `reconciliation.missing` | page | a daily reconciliation has not run for 26 hours (or ever) | at its next run | The worker is probably down: check the background jobs |
| `kra.remittance_due` | warn | withheld tax unremitted for `KRA_REMIT_WARN_AFTER` [72h] (placeholder) | when remitted | Prepare the batch and remit it ([tax](tax.md)) |
| `kra.remittance_overdue` | page | withheld tax unremitted for `KRA_REMIT_DUE_AFTER` [120h] (placeholder) | when remitted | Remit it now |
| `operations.stopped` | page | staff stopped money operations (ADR 0027; [operating mode](operating-mode.md)) | when an admin resumes | Read the reason; fix the cause; an admin resumes on the System page |
| `operations.live_request` | warn | a request to operate live waits for a second admin (one alert per request) | when approved, or after 7 days | A different admin checks the licence evidence and approves, or lets it expire |
| `alerts.check_failed` | page | a check could not run (it then clears nothing it owns) | when the check runs again | Read the error: usually the database or a missing table |

Ages in a summary count from when the thing started waiting (a top-up from when the player paid, a payout from when
it became held or manual). Phone numbers in alerts are masked (`254705***005`); the back office shows them.

## The Alerts page

Back office → Alerts lists every alert, newest first, filtered by state (Open: not resolved, acknowledged or not;
Acknowledged; Resolved; All), kind (with the open count of each) and severity. **Open** shows what the check found,
what to do and where (a link to the page that handles it), who acknowledged or resolved it and when, and the
history (raised, escalated, acknowledged with the note, resolved). Pages and payout freezes also show as a banner on
every page of the back office.

```sh
go run ./cmd/lotteryctl alerts list [-kind topup.unconfirmed] [-state open|acknowledged|resolved|all]   # -all is -state all
go run ./cmd/lotteryctl alerts ack -alert <id> -note "what was found and done" -operator <your back-office email>
go run ./cmd/lotteryctl alerts watch          # run the checks and send what is due now
```

## Payout freeze

A failed reconciliation means the books do not balance, so no money leaves until a person has looked. While a
`reconciliation.failed` alert is open, **no B2C payout is sent**: payouts wait in `pending`, never sent, and go out
automatically once the freeze is closed. Withdrawals can still be requested. Each failed run is its own alert, and a
later clean run does not close it. Only a person closes it:

```sh
go run ./cmd/lotteryctl alerts list              # open alerts
go run ./cmd/lotteryctl draws reconcile          # or payments / game / payouts reconcile: what is wrong
go run ./cmd/lotteryctl alerts ack -alert <id> -note "what was found and done" -operator <alice's back-office email>
```

Acknowledging is audited with the note. For other pages it only stops the reminders; they stay open until their
condition clears (an escalated alert that was acknowledged as a warning is sent once more as a page, but not reminded).

## Channels

- **Webhook** (`ALERT_WEBHOOK_URL`, a secret): each event as JSON `{event, text, alert}`; `event` is `raised`,
  `escalated`, `reminder` or `resolved`.
- **Email** (`ALERT_EMAILS`, comma-separated addresses, not secret): subject `[4draws] PAGE kind: summary`
  (`ESCALATED`, `REMINDER` or `RESOLVED` in front when it is not the first notice), a plain-text body with the
  details and the link `BACKOFFICE_URL/alerts?alert=<id>`. Sent through `SMTP_HOST` like the other staff email; with
  no SMTP the email is only logged.
- A channel that fails is logged and does not stop the others; an event is sent again at the next watch only when no
  channel took it. The System page names the channels in use.

## Heartbeat (dead man's switch)

If the worker stops, nothing above runs. Set `ALERT_HEARTBEAT_URL` to a ping URL of a monitor such as healthchecks.io
(period 5 minutes, grace 15). The watch requests it after every run in which all checks ran and every alert was sent,
and the monitor pages when the pings stop.

## Setting it up (owner)

1. Create the channel (for example a Slack channel with an incoming webhook) and set `ALERT_WEBHOOK_URL` on Render,
   and/or set `ALERT_EMAILS` to the on-call addresses (SMTP is already configured).
2. Create a healthchecks.io check, point its own notifications at a phone (SMS or call), and set `ALERT_HEARTBEAT_URL`.
3. Decide who is on call for pages, and who may acknowledge a payout freeze (finance and engineering together).
4. Once the B2C float is funded, set `B2C_FLOAT_FLOOR_KES` to a day's payouts so the float is topped up in time.
