# Repository audit (baseline, 2026-11-07)

This is the Phase 0 audit of the engagement in `docs/release/implementation-roadmap.md`. It describes the repository as it
is on `main` at `9e11bf4`. Every finding cites the file it comes from. Findings were read from the code, not from the
docs; where the code and a doc disagree, the code is quoted.

Companion documents: `critical-user-journeys.md` (this folder), `docs/security/security-audit.md`,
`docs/release/mobile-release-audit.md`, `docs/compliance/compliance-gap-analysis.md`.

## 1. Layout

| Path | What it is |
|---|---|
| `cmd/api` | Public HTTP API; runs the workers in-process when `WORKERS_IN_PROCESS=true` (`internal/platform/config/config.go:52`) |
| `cmd/worker` | The same workers as a separate process (`cmd/worker/main.go:50`) |
| `cmd/lotteryctl` | Operator CLI: migrations, KYC decisions, payments recovery, draws, alerts |
| `internal/*` | Modules of the monolith (below) |
| `db/migrations` | 44 goose migrations, embedded (`db/migrations/embed.go`) |
| `db/queries` | sqlc queries; generated stores in `internal/*/store` |
| `api/openapi.yaml` | Public API contract (linted in CI) |
| `apps/mobile` | Flutter player app: Android, iOS, web |
| `apps/backoffice` | React 19 + Vite staff back office |
| `apps/web` | Empty (`.gitkeep` only) |
| `tools/verifier` | Independent re-implementation of spin and draw verification |
| `tools/erd` | ERD generator (`docs/erd.svg`) |
| `loadtest` | Load test scripts |
| `deploy`, `render.yaml`, `Dockerfile`, `docker-compose*.yml` | Render Blueprint and local stack |
| `docs/adr` | 26 ADRs (0001–0026) |
| `docs/runbooks` | Operator runbooks |
| `docs/checkpoints` | Phase checkpoints 0–8 |

## 2. Backend modules

| Module | Responsibility | Notes |
|---|---|---|
| `identity` | Accounts, phone+password and Google/Apple sign-in, sessions, devices, eligibility asserts | `eligibility.go` holds every money-path check |
| `kyc` | National ID submission, manual review (ADR 0009), under-18 refusal at submission | Date of birth optional under manual review (`kyc.go:177`) |
| `rg` | Deposit limits, time-outs, self-exclusion; append-only, never shortened | `rg.go` |
| `wallet` | Balances: conditional UPDATE + append-only transaction + balanced journal in one tx | `wallet.go:145-185` |
| `ledger` | Double-entry posting rules, integer KES (ADR 0001) | DB-enforced balance (`00004`) |
| `payments` | STK top-ups, C2B paybill (shelved, ADR 0011), reversals, reconciliation, SSE events | Callback trust issues, see security audit |
| `payouts` | B2C withdrawals and prizes, KRA withholding, payout reconciliation, payout freeze | DB state machine (`00023`) |
| `game` (+`game/fair`) | Plays: price, ticket, provably fair spin | HMAC-SHA256 with rejection sampling (`fair.go:46-97`) |
| `tickets` | Tickets and draw entries | |
| `draws` | Personal-period draws (ADR 0017), drand quicknet, two officials sign and approve | `draws.go` |
| `finance` | Finance dashboard and exports (ADR 0023) | |
| `alerts` | Operational alerts, webhook, heartbeat, payout freeze (ADR 0025) | |
| `audit` | Append-only audit log written in the action's transaction | |
| `oversight`, `metrics` | Back-office read models | |
| `staff`, `backoffice` | Staff identity, two roles `admin`/`staff` (ADR 0019) | |
| `notifications` | SMS sender, unused since ADR 0005 | |
| `platform/*` | config, db, crypto, idempotency, ratelimit, telemetry, mail, challenge (attestation), clock | |
| `api` | The only HTTP layer; routes in `internal/api/api.go` | |
| `app` | Wiring for api and worker (`internal/app/app.go`) | |

Workers and periodic jobs are registered unconditionally in `app.StartWorkers` (`internal/app/app.go:370-395`).

## 3. Data and money

- **Money** is whole KES in `bigint` (ADR 0001). Balanced journals are checked in Go (`internal/ledger/ledger.go:62-90`)
  and by deferred constraint triggers (`db/migrations/00004_ledger_wallet.sql:65-91`). No double posting:
  `UNIQUE (kind, reference_type, reference_id)` (`00004:35`).
- **History is append-only**: `forbid_mutation` triggers on journals, entries and wallet transactions (`00004:93-100`,
  `:142-145`), tested even against the table owner (`internal/platform/db/schema_integration_test.go:203`).
- **Balances** cannot go negative (`00004:104`) and must equal the sum of wallet transactions (`00004:122-140`).
- **Gap:** `app_api` and `app_worker` keep UPDATE on `wallet.wallets` (`00008_grants.sql:27`). In single-role mode,
  which production currently runs (`docs/STATUS.md`), the grants are skipped altogether (`00008:17-21`).

## 4. Authentication and authorisation

- Players: EdDSA access JWT, 10 min (`internal/app/app.go:192`); refresh tokens with sessions. Each request checks the
  session's `revoked_at`, not `users.status` (`internal/identity/session.go:315-342`). Suspension and closure revoke all
  sessions (`internal/identity/account.go:243`, `:123`), so they take effect at once.
- Staff: email+password, optional Google/Apple; roles `admin` and `staff` (`internal/staff`, ADR 0019), checked
  in the API on every call.
- Device attestation: challenges plus App Attest and Play Integrity verifiers (ADR 0014); the apps wire only a no-op
  attestor (`apps/mobile/lib/core/platform/device_attestation.dart:13-21`).

## 5. Player app (Flutter)

- go_router with a session guard and five tabs (`apps/mobile/lib/app/router.dart`); Riverpod 3; `core/api/api_client.dart`
  with idempotency keys; access token in memory, refresh token in Keychain/Keystore (`core/auth/token_store.dart`).
- The wheel lands on the server's slot only (`features/play/presentation/widgets/lottery_wheel.dart:13-35`); reduced
  motion is honoured (`app/theme/tokens.dart:254`).
- No analytics, crash-reporting, ads or push SDKs (`apps/mobile/pubspec.yaml:36-45`).
- 24 test files, ~230 cases including goldens, one integration journey (`integration_test/play_journey_test.dart`).

## 6. Back office (React)

Pages: Overview, Finance, Metrics, Players, Plays, TopUps, Kyc, Draws, Payouts, Payments, Tax, Alerts, System, Activity,
Closures, Staff (`apps/backoffice/src/pages`). Role gating in `src/roles.ts:10-41`. Self-exclusion is view-only
(`pages/Players.tsx:375-379`); held payouts are approved by one click by one person (`pages/Payouts.tsx:58`).

## 7. Delivery

- `ci.yml`: Go lint (golangci-lint, vet, staticcheck@latest), gen-check, openapi-lint, migrations up/down/up, unit,
  integration, property, coverage, skipped-tests, backoffice lint/typecheck/test, Flutter format/analyze/test/web build,
  e2e, gitleaks, govulncheck, osv-scanner (`go.mod` only), Trivy fs.
- The `web` job targets `apps/web`, which has no `package.json`, so it never runs.
- `mobile.yml`: unsigned APK/AAB on PRs (Android release signed with the debug key), iOS `--no-codesign` on tags. No
  store upload.
- `images.yml`: images, Trivy, SBOM, cosign. `deploy-prod.yml`: Render deploy on tags behind the `production`
  environment. `nightly.yml`: mutation tests (gremlins, 75%), long property runs, a 10M-spin distribution test.

## 8. Baseline results (this machine, 2026-11-07)

| Check | Result |
|---|---|
| `go build ./...` | pass |
| `make lint` (gofmt, vet, golangci-lint 2.15.0, actionlint) | pass, 0 issues |
| `make test` (unit, `-race -shuffle`) | pass |
| `make test-int`, `make e2e`, `make cover` | **not run**: the Docker daemon is not running on this machine; CI runs them on every PR |
| `apps/backoffice` `npm run lint`, `npm run typecheck` | pass |
| `apps/backoffice` `npm test` | 9 tests timed out at 15 s in a full parallel run on this 2-CPU machine (`oversight.test.tsx`, `pages.test.tsx`); rerun alone, both files pass (30/30). Environmental, not a regression; CI passes |
| `apps/mobile` `flutter analyze` / `flutter test` | **not run**: Flutter 3.47.7 is not installed here; CI runs them on every PR |

Pre-existing failures: none observed in what could be run.

## 9. Status of capabilities

Legend: **T** implemented and tested · **P** partial · **D** documented, not implemented · **N** not implemented.

| Capability | Status | Evidence |
|---|---|---|
| Double-entry ledger, append-only history | T | `00004`, `ledger_test.go:496`, `schema_integration_test.go` |
| STK top-up with idempotency and duplicate-callback safety | T | `payments_integration_test.go:427` |
| Late/orphan callback crediting **without M-Pesa verification** | P (defect) | `payments.go:456-460`, `reconcile_topups.go:264-295` |
| B2C payouts with DB state machine, never re-sent | T | `00023`, `TestWithdrawal_FaultMatrix` |
| Provably fair spin, server-authoritative | T | `game/fair`, `game_integration_test.go`, `tools/verifier` |
| Draws with drand and two officials | T (API) / P (CLI: free-text official) | `draws.go`, `cmd/lotteryctl/main.go:486` |
| Per-player eligibility on money in and play | T | `identity/eligibility.go`, tests in §2 of the compliance gap analysis |
| Global operating gate / kill switch | N | no flag in `internal/platform/config` |
| Server-side age evidence before money in | N | 18+ checkbox is client-only (`sign_in_screen.dart:60`) |
| Deposit limits, time-out, self-exclusion | T | `rg_integration_test.go` |
| Play/loss/session limits | N | — |
| Geolocation | N | — |
| AML monitoring and cases | N | — |
| Account deletion | P | request plus staff close; no data deletion (ADR 0015 proposed) |
| Data retention | D | ADR 0015 "proposed" |
| Android release signing | N | debug key (`build.gradle.kts:36`) |
| iOS signing, privacy manifest | N | no team, no `PrivacyInfo.xcprivacy` |
| Policy pages (privacy, terms, deletion) | N | `apps/web` empty |
