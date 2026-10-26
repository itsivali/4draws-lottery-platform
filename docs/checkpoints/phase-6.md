# Phase 6 — Player web app and back office: checkpoint

Status: IN PROGRESS (started 2026-10-26 on the owner's instruction: "phase 6 on the web, and apps, also cover back
office operations and continue with phase 7 and 8").

## Decisions taken (ADR 0012)

- **One Flutter app for players** on iOS, Android and the web (`apps/mobile`); its web build is the player web app.
  `apps/web` stays empty. The staff back office is a separate app (`apps/backoffice`).
- **Placeholders for the owner:** the brand name and wordmark ("Bahati", `BrandMark`), the app id
  `ke.bahati.lottery_mobile`, and the icon (Flutter's default) are placeholders until the owner chooses them.

## Plan (written before coding)

Rules across every slice:
- The API decides every outcome, balance and payment; clients only show its answers. No client ever generates an
  outcome, assumes a payment, or guesses an endpoint.
- Every write carries an idempotency key; an uncertain write is retried with the same key, never with a new one.
- Tests first (Go: `-race -shuffle`; Flutter: unit, widget, golden and journey tests against a fake of the API).

### Slice 1 — API for the player clients
`GET /v1/wallet`, `GET /v1/account`, `GET /v1/payments`, `POST /v1/account/closure` (migration 00027), CORS by exact
origin (`CORS_ALLOWED_ORIGINS`). All in the auth and idempotency sweeps; OpenAPI updated.

### Slice 2 — The player app (Flutter)
Design system (`design-system/lottery-platform/`), session and API layers, the wheel, play, draws, wallet, account,
responsible gaming, fairness; responsive (phone, tablet, desktop web); accessible; tests.

### Slice 3 — Back-office API
Staff sign-in with Keycloak (OIDC, local stack) and roles; staff routes over the operations `lotteryctl` performs
today (KYC review, suspense and reconciliation, draw sign-off and approval, payout approval, account closure), every
action audited as staff.

### Slice 4 — Back office (`apps/backoffice`, TypeScript)
The screens over slice 3, as CI already expects (`npm run lint`, `typecheck`, `test`).

## Results

### Slice 1 (API for the player clients) — branch `feat/player-account-api`
- `GET /v1/wallet`: balance, debt and transactions in one read-only repeatable-read snapshot; cursor pages
  (`next_before`); a bad cursor is a 400.
- `GET /v1/account`: number, phone proof, KYC, status, the restriction in force (a permanent self-exclusion wins,
  else the longest), closure request.
- `GET /v1/payments`: top-ups newest first, pending ones included.
- `POST /v1/account/closure`: one request per player, kept; staff close it once (trigger); audited once.
- CORS: exact `https` origins (`http` only on localhost), no wildcard, no credentials, off when unset.
- Tests: API journeys and ownership for each route, closure invariants in the database, CORS unit tests, config
  validation. Full Go unit and integration suites pass.

### Slice 2 (player app) — branch `feat/mobile-app`
- **Design system** made with the UI/UX Pro Max skill (installed for Claude Code in a scratch directory; its style,
  type and anti-patterns kept, its amber/purple palette replaced: see MASTER.md *Provenance*). Tokens in
  `lib/app/theme/tokens.dart`; every text/surface pair checked for WCAG AA contrast by a test.
- **Wheel:** pure geometry (`WheelGeometry`), a time-based motion plan (`WheelMotion`), painters in three layers, an
  animation host that lands on the server's slot; reduced motion settles without spinning; resize-safe.
- **Haptics:** one `Haptics` service; `SpinHaptics` ties ticks to real pointer crossings while slowing, spaced and
  capped; off on the web and behind a setting that loads before the first frame.
- **Play:** `PlayController` state machine (idle → submitting → landing → revealed; refused; uncertain with
  reconciliation by the same key, bounded; recovery at launch, bound to the owner's user id); eligibility from the
  server's state; confirmation of the price before every play; result copy that never says "win".
- **Wallet:** `SettlementFlow` for top-ups and withdrawals: one key, pending until the server's final state, bounded
  backoff, safe to leave; a second top-up cannot start while one is pending.
- **Account:** KYC, responsible gaming (limits, breaks, self-exclusion; Responsible Gambling Kenya listed as Google
  Play requires), password, closure request, haptics setting, sign-out. **Fairness:** revealed spins recomputed on
  the device from the shared test vectors' specification.
- **Tests (all passing):** 223 — geometry and motion, haptics, API client, session, models, verifier (shared
  vectors), controller, settlement, eligibility, formatting, contrast, wheel widget and goldens, whole-app flows
  (sign-in, play, lucky result, lost answer recovered, refusals, top-up, 200 % text on every tab, keyboard,
  semantics, reduced motion, haptics off, session end), screen goldens, and the main journey (also an on-device
  integration test). `flutter analyze` and `dart format` clean. `flutter build web --release` passes; the build was
  rendered in Chromium.

### Slice 3 (back-office API) — branch `feat/mobile-app` (stacked)
- Staff identity (ADR 0013): `identity.OIDCVerifier.VerifyAccessToken` (no nonce; returns the claims) and
  `internal/staff` (roles from `realm_access.roles`, uuid subject, unknown roles ignored, none refused). Staff appear as
  `staff:<sub>` in audits and operator fields, so the existing two-person rules compare real people.
- `/v1/staff/*`: 31 routes over every operator action of `lotteryctl` (KYC review; payments reconciliation, reversals,
  unconfirmed and suspense; draws sign, approve, void, overdue, reconciliation; game reconciliation; payouts approve,
  reject, settle, retry prizes, reconciliation; KRA batches, file, remittance; alerts; account closures). Each route
  needs a role; staff tokens are authenticated before idempotency, so keys are scoped per staff member.
- The back office acts through services on `STAFF_DATABASE_URL` (worker role): the player API's role still cannot
  change draws or payouts. Migration 00027's grants corrected: the API inserts closure requests, the worker closes them.
- Account closure: `identity.CloseAccount` locks the request, refuses while the wallet holds money or a top-up or
  payout is unsettled (checked in the same transaction), closes the account, ends every session, audits.
- Local Keycloak staff realm (`deploy/keycloak`) imported by `make dev`; verified against a running Keycloak 26: a
  real access token (issuer, audience, uuid subject, roles) passes the staff authenticator.
- Tests: the verifier and roles (unit), config, and the staff API (every route refuses missing, player and garbage
  tokens, a member with no role or the wrong role, and writes without a key; KYC review end to end with audit; a draw
  signed only by two different officials; closure blocked by balance and by an unsettled payout, then closed and the
  session ended; 503 when unconfigured). OpenAPI documents every route (`back-office` tag, `staffBearer`).

### Slice 4 (back office app) — `apps/backoffice`
- React 19 + TypeScript + Vite; OIDC sign-in (PKCE, session storage); roles from `GET /v1/staff/me`; sections and
  buttons follow the roles. Pages: overview (and reconciliations), identity review, draws (signature counts, late
  steps), payouts (by state), payments (reconciliations and actions), KRA tax, alerts, account closures.
- Every write: confirmation with the reason or reference the audit needs; one idempotency key, retried once on a lost
  answer; unknown outcomes shown as such with *Check again*; refusals in the API's words with "Nothing was changed".
- Design system tokens as CSS variables (light and dark), tabular figures, visible focus, skip link, status pills with
  symbol and word, reduced motion respected.
- `npm run lint`, `typecheck`, `test` (14 tests: API client, roles and routing, identity review, two-official refusal,
  auditor read-only, lost answer checked again with the same key, closure refusal, config) and `build` pass.

## Release readiness (honest status)

| | Status |
|---|---|
| Implemented | Everything in slices 1–4 above |
| Tested and passed | Go unit + integration suites; Flutter analyze, format, tests, goldens; web release build; back office lint, typecheck, tests, build; local Keycloak token verified by the staff authenticator |
| Tested and failed | — |
| Not tested | The back office in a browser against the running stack (component tests only); Android and iOS builds (no Android SDK or macOS here; the `mobile` workflow builds them in CI); haptics on physical phones; screen readers (TalkBack, VoiceOver) on devices; performance on low-end phones; the app against the live Render API |
| Needs backend work | Device attestation (phase 7); push notifications (phase 7); a real-time stream on the web (the app polls `GET /v1/payments/{id}`; SSE through `fetch` is a follow-up); the chance-tier table and prize shares are not published by the API, so the app does not state them |
| Needs owner / legal / store | See below |

**Release blockers (owner, counsel, stores):**
- **Licensing:** a BCLB/GRA licence for the lottery (Gambling Control Act, 2025) and any approval of the phone-proof
  model (ADR 0005). Nothing in the app may be distributed publicly before it.
- **Apple App Store (5.3.4, 5.3.3, 5.1.1(v)):** licence and permissions where used; geo-restricted to Kenya; free on
  the store; no in-app purchase for credits (M-Pesa top-ups only); submitted by the licensed legal entity
  (5.1.1(ix)); account deletion must be offered in the app (the app offers a closure request; staff closing it in the
  back office, slice 3–4, must exist and the retention rule must be decided).
- **Google Play (real-money gambling):** Kenya is an allowed country for lotteries, subject to Google's application and
  licensing requirements; the listing needs a responsible-gambling statement and the app shows local support
  (done: Responsible Gambling Kenya).
- **Brand, app id, icon, signing:** placeholders; Android release signing and iOS signing are not configured.
- **Privacy:** privacy policy, data-safety form (Play) and privacy labels (Apple); data retention for closed accounts.
- **Web:** set `CORS_ALLOWED_ORIGINS` on Render to the web app's origin; host the web build (Render static site).
- **Staff sign-in in production:** a Keycloak (or equivalent) for staff, with MFA, an administrator who grants roles,
  and at least two `draw_official` and two `tax_officer` people. Not on Render's free plan today.
