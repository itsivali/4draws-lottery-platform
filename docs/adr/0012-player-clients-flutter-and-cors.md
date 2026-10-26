# 0012. One Flutter app for players (iOS, Android, web); CORS by exact origin

Status: accepted
Date: 2026-10-26

## Context

Phase 6 needs a player web app and phase 7 the phone apps. The repository reserved `apps/web` (npm) and
`apps/mobile` (Flutter). The owner asked (2026-10-26) for the player experience to be one Flutter app on iOS, Android
and the web, and for the back office to come in phase 6 as well.

The API had never been called from a browser: it sent no CORS headers, so a web app on another origin could not call
it. The clients also needed reads the API did not offer: the wallet balance and its history, the account's
verification and restriction state, the list of top-ups, and a way to ask for the account to be closed (the app stores
require a way to start account deletion from inside the app).

## Decision

- The player app is `apps/mobile` (Flutter), built for iOS, Android and the web. `apps/web` stays empty; its CI job
  only runs if files appear there. The staff back office is a separate app (`apps/backoffice`).
- `CORS_ALLOWED_ORIGINS` lists the exact origins that may call `/v1` from a browser. Entries must be `https`
  (`http` only on `localhost`/`127.0.0.1`); wildcards, paths and credentials are refused at start-up. Empty (the
  default) means no CORS at all, so the API stays closed to browsers until an origin is named. No credentials mode:
  tokens travel in the `Authorization` header, never in cookies.
- New player reads: `GET /v1/wallet` (balance, debt, transactions, cursor pages, one snapshot), `GET /v1/account`
  (number, phone proof, KYC, status, the restriction in force, closure request), `GET /v1/payments` (top-ups, newest
  first). New write: `POST /v1/account/closure` records a request (`identity.closure_requests`, one per player,
  audited). Closing is a staff step: the balance is paid out first and the records the law requires are kept.

## Consequences

- One codebase for three platforms; the design system (`design-system/lottery-platform/MASTER.md`) applies to all.
- The web app's origin must be added to `CORS_ALLOWED_ORIGINS` on Render before it can work.
- Account closure needs a back-office screen (phase 6) and a data-retention rule for closed accounts (phase 8; owner
  and counsel).
