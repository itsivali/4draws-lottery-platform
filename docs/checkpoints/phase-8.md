# Phase 8 — Production hardening and launch: checkpoint

Status: IN PROGRESS (2026-10-26). The code-side hardening below is done; launch needs the owner (paid plans,
credentials, licence, stores).

## Done

### Independent security review of phases 6–7 (fixed)
A separate reviewer read the diff with the threat model and no conclusions from the author. Findings and outcomes:

| Severity | Finding | Outcome |
|---|---|---|
| High | Closing an account while it still had chances or prizes in draws would strand winnings (a closed account cannot be paid) | **Fixed:** closure refuses while the player has paid tickets in an open draw's period, entries in a draw not yet paid, or a prize without a successful payout, and while debt is owed. Tests. |
| High | `ATTESTATION_MODE=require` passes any client that calls itself "web" | **Fixed as a decision:** `ATTESTATION_WEB=allow|refuse`; documented that the strict modes cover phone clients unless the web is refused, and that a money-moving web app needs its own controls (ADR 0014). Test. |
| Medium | One person could meet a two-person rule by acting once in the back office and once with `lotteryctl` (two different operator strings) | **Fixed:** `lotteryctl -operator` now takes the staff id (Keycloak subject) and records `staff:<id>`, the same identity as the back office. Runbooks updated. Tests. |
| Medium | Attestation is checked at sign-in only; copied tokens keep a passed verdict | **Documented** as the follow-up (App Attest assertions, request-bound Play Integrity on withdrawals) in ADR 0014. |
| Low | A top-up could interleave with closing the account | **Fixed:** eligibility reads hold the account row (`FOR SHARE`); closing locks it (`FOR UPDATE`). |
| Low | Listing closure requests showed phone numbers without an audit record | **Fixed:** audited (`identity.closures_viewed`). Test. |
| Low | The local realm's password-grant e2e client must never reach production | **Documented** in the back-office runbook. |

Areas the reviewer checked and found sound: staff authentication and role checks, token validation, audit attribution,
idempotency scoping, the new player routes (no IDOR), CORS, challenge handling, both verifiers, KYC unmask auditing,
token storage in both clients, server-decided outcomes and same-key retries.

### Dependency audit
`apps/backoffice`: react-router-dom upgraded past open-redirect/XSS advisories; vite and vitest upgraded; `npm audit`
reports 0 vulnerabilities.

### Load test (`loadtest/`)
`go run ./loadtest` runs the player journey with many players at once and checks the money invariants afterwards
(at most 4 plays a day, a replay charged once, every wallet = credited − plays). Local result: 100 players, 25 at a
time, 400 plays out of 700 concurrent attempts, invariants held; latencies in `loadtest/README.md`.

### End-to-end journey (`make e2e`, CI)
Sign-up, KYC, top-up, paybill, play, withdrawal confirmed and paid, attestation challenge, back office with real staff
tokens (phase 7).

### Runbooks and decisions
`docs/runbooks/incidents.md` (payments, draws, security, data); ADR 0015 data retention (proposed: needs counsel).

## Needs the owner before launch

| Item | Why |
|---|---|
| Licence (BCLB/GRA) and approval of the phone-proof model (ADR 0005) | No real money before it |
| Paid Render plans: database with backups, separate worker; the `app_api` / `app_worker` login roles and `STAFF_DATABASE_URL` | Free Postgres expires and runs in single-role mode; the back office should act with the worker's role |
| Production Daraja credentials, paybill/till, `DARAJA_ALLOW_PRODUCTION` | Real money |
| A production staff realm (Keycloak or equivalent) with MFA, an administrator, two draw officials and two tax officers | Back office sign-in; two-person rules |
| Apple and Google developer accounts; App Attest and Play Integrity; then `ATTESTATION_MODE=record` → `refuse_failed` | Device trust (phase 7) |
| Hosting for the player web build and the back office; `CORS_ALLOWED_ORIGINS` | Web clients |
| Retention periods (ADR 0015), privacy policy, store listings and data-safety forms | Data protection; stores |
| A load test on the production plan; alert channel and on-call (`docs/runbooks/alerts.md`) | Capacity; response |
| Responsible-gambling statement for store listings; confirm support contacts | Google Play policy |
