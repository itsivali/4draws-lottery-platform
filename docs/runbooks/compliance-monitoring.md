# Transaction monitoring (ADR 0033)

The rules run every hour and open cases on the back office's **Compliance** page. They are a starting point chosen by
engineering. Until the compliance owner approves each rule's thresholds, the page shows *pending compliance-owner
approval*.

## Working a case

1. **Open the player** from the case. Their timeline shows top-ups, plays, payouts, sign-ins and where they came from.
2. **If money may need stopping while you look,** use **Hold money** with the reason. Top-ups, paybill payments,
   withdrawals and payouts stop; play does not. **Never tell the player why**: the app only says it is not available
   right now.
3. **Add notes** as you find things.
4. **Close the case** with the outcome: what you decided, and whether it was reported to the Financial Reporting Centre
   (reports are made outside the platform). Closing lifts the hold.

Someone other than the person who placed a hold lifts it or closes the case. Closed cases are final.

To open a case yourself, ask an engineer (the API: `POST /v1/staff/aml/cases` with the player and why); the back office
has no form yet.

## Rules and thresholds

```sh
go run ./cmd/lotteryctl aml rules
go run ./cmd/lotteryctl aml set-rule -rule deposit_velocity \
  -params '{"window_hours":24,"max_kes":100000,"max_count":10,"quiet_days":30}' \
  -note "thresholds approved in the compliance meeting of <date>" -approval "<approval reference>" \
  -operator <your back-office email>
go run ./cmd/lotteryctl aml set-rule -rule shared_device -params '{"min_accounts":3,"quiet_days":90}' -off \
  -note "families share phones; off until reviewed" -operator <your back-office email>
go run ./cmd/lotteryctl aml scan        # run the rules now
```

Each rule's parameters:

| Rule | Parameters |
|---|---|
| `deposit_velocity` | `window_hours`, `max_kes`, `max_count`, `quiet_days` |
| `in_out_little_play` | `window_hours`, `min_withdrawal_kes`, `max_play_percent` (1–100), `quiet_days` |
| `failed_topups` | `window_hours`, `min_failed`, `quiet_days` |
| `shared_id_number` | `min_accounts` (2 or more), `quiet_days` |
| `shared_device` | `min_accounts` (2 or more), `quiet_days` |

Every change is a new version, kept with who made it and why.
