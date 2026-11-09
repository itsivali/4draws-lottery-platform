# Operating mode

ADR 0027. The operating mode says whether money moves and whether it is real:

| Mode | What happens |
|---|---|
| `disabled` | No top-up, play, withdrawal, approval of a held payout or payout send starts. STK callbacks, verification, paybill confirmations, reversals, reconciliation and operator recovery go on, so no player's money is stranded. Draws go on; prizes are recorded and their payouts wait. |
| `sandbox` | Everything works with sandbox money (Daraja's sandbox or the local mock). No real stakes or prizes. |
| `live` | Real money. Needs production Daraja, `REGULATORY_LICENCE_REFERENCE`, and two admins (below). |

The effective mode is the lower of:

1. `OPERATING_MODE` on the api (default `disabled`). The api refuses to start with `sandbox` and production Daraja, or
   with `live` without production Daraja or `REGULATORY_LICENCE_REFERENCE`.
2. What staff recorded (`compliance.mode_changes`); `sandbox` when nothing is recorded. `live` counts only through an
   approved live request naming the configured licence.

`GET /v1/status` shows the effective mode to anyone; the back office shows it at the top of **System status**, and
`lotteryctl compliance status` prints what is recorded.

## Stop everything (anyone on staff)

Back office → System status → **Stop money operations**, with a reason. Or, without the back office:

```sh
lotteryctl compliance stop -reason "what happened" -operator <your staff id>
```

The `operations.stopped` alert pages until an admin resumes. Players see "Top-ups, plays and withdrawals are paused";
their balances are untouched.

## Resume (an admin)

Back office → System status → **Resume (sandbox)**, with what was found. Resuming never makes the service live.

## Go live (two admins, after licensing)

Only when the licence is granted, the regulator's conditions are met (`docs/compliance/evidence-register.md`) and the
store approvals exist:

1. Set on the api: production Daraja (`DARAJA_BASE_URL=https://api.safaricom.co.ke`, `DARAJA_ALLOW_PRODUCTION=true`,
   production credentials), remove `DARAJA_SANDBOX_SETTLE`, set `REGULATORY_LICENCE_REFERENCE`,
   `GEO_COUNTRY_HEADER` (the edge's country header, `CF-IPCountry` behind Cloudflare, ADR 0031; the service refuses to
   start live without it) and `OPERATING_MODE=live`. Deploy. The service is now `disabled`: nothing moves.
2. Admin 1: System status → **Request to go live**: the licence reference (exactly as configured), the regulator's
   approval reference, the SHA-256 of the evidence file (`sha256sum licence.pdf`) and where the evidence is kept.
3. Admin 2 (a different person): checks the evidence against the hash, then **Approve**. The service is live at once.

A request expires after 7 days. Requests, approvals and changes are append-only and in the activity log. A
configuration change alone, or a database row alone, never makes the service live.
