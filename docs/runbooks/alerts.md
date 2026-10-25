# Runbook: alerts

Every 5 minutes the worker's `alerts.watch` runs the checks below. Each condition becomes **one** alert in
`audit.alerts` however often it fires. New alerts are sent to `ALERT_WEBHOOK_URL`, a Slack-compatible incoming webhook
that gets JSON with a `text` field. Pages are sent again every hour until someone acknowledges them. When an alert's
condition clears, it is resolved and the resolution is sent too. Without a webhook, alerts are logged
(`ERROR alert`) and kept in the database.

| Alert | Severity | Raised when | Clears |
|---|---|---|---|
| `draw.overdue` | page | a draw's next step is late (`lotteryctl draws overdue`; see [draws](draws.md)) | when the draw moves on |
| `reconciliation.failed` | page, **freezes payouts** | the latest payments, game, draws or payouts reconciliation found mismatches | only when a person acknowledges it |
| `reconciliation.missing` | page | a daily reconciliation has not run for 26 hours (or ever) | at its next run |
| `kra.remittance_due` | warning | withheld tax unremitted for 72 h (placeholder) | when remitted |
| `kra.remittance_overdue` | page | withheld tax unremitted for 120 h (placeholder) | when remitted |
| `alerts.check_failed` | page | a check could not run (it then clears nothing it owns) | when the check runs again |

## Payout freeze

A failed reconciliation means the books do not balance, so no money leaves until a person has looked. While a
`reconciliation.failed` alert is open, **no B2C payout is sent**: payouts wait in `pending`, never sent, and go out
automatically once the freeze is closed. Withdrawals can still be requested. Each failed run is its own alert, and a
later clean run does not close it. Only a person closes it:

```sh
go run ./cmd/lotteryctl alerts list              # open alerts (-all for history)
go run ./cmd/lotteryctl draws reconcile          # or payments / game / payouts reconcile: what is wrong
go run ./cmd/lotteryctl alerts ack -alert <id> -note "what was found and done" -operator alice
```

Acknowledging is audited with the note. For other pages it only stops the reminders; they stay open until their
condition clears. `lotteryctl alerts watch` runs the checks immediately.

## Heartbeat (dead man's switch)

If the worker stops, nothing above runs. Set `ALERT_HEARTBEAT_URL` to a ping URL of a monitor such as healthchecks.io
(period 5 minutes, grace 15). The watch requests it after every run in which all checks ran and every alert was sent,
and the monitor pages when the pings stop.

## Setting it up (owner)

1. Create the channel (for example a Slack channel with an incoming webhook) and set `ALERT_WEBHOOK_URL` on Render.
2. Create a healthchecks.io check, point its own notifications at a phone (SMS or call), and set `ALERT_HEARTBEAT_URL`.
3. Decide who is on call for pages, and who may acknowledge a payout freeze (finance and engineering together).
