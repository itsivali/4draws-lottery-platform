# Security audit (baseline, 2026-11-07)

Scope: the Go API, payment hooks, staff operations and the player apps, read from source on `main` at `9e11bf4`.
Severity reflects real-money operation; the deployed service runs only the Daraja sandbox today (`render.yaml:95-108`).

| ID | Severity | Finding | Evidence | Planned fix |
|---|---|---|---|---|
| SEC-01 | High | A paid STK callback for a top-up that already settled uncredited is credited straight from the callback's amount and receipt, with no Transaction Status check. The operator `Recheck` of orphan callbacks does the same. This contradicts ADR 0006 (the callback is a claim, the status query is the proof). Anyone who holds the callback secret could credit an expired top-up. | `internal/payments/payments.go:456-460`, `internal/payments/reconcile_topups.go:264-295`, `:352-361` | S2: fixed (ADR 0028) |
| SEC-02 | High | On the normal path the status query proves a payment happened, but the credited amount and receipt come from the stored callback. `RecordCallback` overwrites the stored callback on every call, so a second, forged callback supplies the amount. Nothing compares it with `amount_requested_kes`. | `db/queries/payments.sql:28-32`, `payments.go:597-639`, `payments_integration_test.go:500` | S2: fixed (ADR 0028) |
| SEC-03 | Medium | The hook secret (one segment shared by the STK, C2B and B2C hooks) is the URL path. The OTEL span name is `r.URL.Path` and the client-IP diagnostic logs the path, so the secret reaches traces and logs. There is no IP allowlist. | `internal/platform/telemetry/telemetry.go:85`, `internal/platform/httpx/clientipdiag.go:18`, `internal/api/api.go:83-93` | S2: fixed (ADR 0028; IP allowlist for SEC-03 deferred to S12) |
| SEC-04 | Medium | `payments.payments` has no DB transition trigger; `SetPaymentOutcome` and `MarkCredited` have no status guard. Payouts and draws do have triggers. | `payments.sql:34-44`; compare `00023_payouts.sql:35-61` | S2: fixed (ADR 0028) |
| SEC-05 | Medium | Single-person operator powers: approving a held payout, `Settle(paid)` with an unchecked receipt, and `Settle(not paid)` (a double payout if B2C actually paid). | `internal/payouts/operator.go:49-101` | S4: fixed (ADR 0032; a settled receipt is confirmed by M-Pesa Transaction Status, ADR 0034) |
| SEC-06 | Medium | `lotteryctl` draw signing takes the official from a free-text `-operator` flag, so one person with `DATABASE_URL` can act as both officials. | `cmd/lotteryctl/main.go:486-488` | S4: fixed (ADR 0032; residual: database access itself, see the ADR) |
| SEC-07 | Medium | No global switch stops real-money operations; plays cannot be stopped at all. | `internal/platform/config/payments.go`, `internal/app/app.go:214,370-395` | S1 |
| SEC-08 | Low | Production runs in single-role DB mode, so the least-privilege grants of `00008` are not applied. Even with them, `app_api` and `app_worker` keep UPDATE on `wallet.wallets`. | `00008_grants.sql:17-27`, `docs/STATUS.md` | Later (paid DB plan) |
| SEC-09 | Low | `fairness/rotate` creates the new server seed after the client seed arrives in the same request, which in principle allows the operator to grind seeds. | `internal/game/game.go:440`, `:272-289` | Backlog |
| SEC-10 | Low | `RequestWithdrawal` reads debt without `LockDebt`; a concurrent reversal can leave debt (recovered later). | `internal/payouts/payouts.go:272` | Backlog |
| SEC-11 | Low | Android `allowBackup` defaults to true with no extraction rules. | `apps/mobile/android/app/src/main/AndroidManifest.xml` | S6 |
| SEC-12 | Info | Native device attestation is not wired (no-op attestor), so `ATTESTATION_MODE` cannot require it. | `apps/mobile/lib/core/platform/device_attestation.dart:13-21` | Owner (Apple/Google accounts) |

## Strengths observed

- Append-only, DB-enforced ledger and wallet history; balances cannot go negative or drift from their transactions.
- Idempotency on every money route; per-player advisory locks; unique receipts and conversation ids.
- Payouts are committed as `sending` before the B2C call and are never re-sent.
- Session revocation on suspension, closure and password reset; refresh tokens in Keychain/Keystore.
- CI: gitleaks, govulncheck, osv-scanner (Go), Trivy fs and image scans, cosign-signed images, SBOM.
- No analytics, ads or crash SDKs in the app.

## Not yet assessed

Penetration testing of the deployed service, back-office XSS/CSRF review, dependency review of the Flutter and npm
trees (no scanner covers them; S9), and secrets on Render (owner).
