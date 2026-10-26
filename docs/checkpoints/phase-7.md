# Phase 7 — Mobile apps and device trust: checkpoint

Status: BUILT where code alone can do it (2026-10-26); the rest needs the owner's developer accounts.

The Flutter app itself (iOS, Android and web from one codebase) was built in phase 6, slice 2 (ADR 0012,
`docs/checkpoints/phase-6.md`). Phase 7 adds device trust and the end-to-end journey.

## Plan (written before coding)

- Attestation tokens are bound to a server-issued, single-use challenge; verdicts come only from server-side
  verifiers; a mode lets operators move from recording verdicts to refusing untrusted devices (ADR 0014).
- Verifiers testable without Apple or Google: App Attest against an injectable root (Apple's embedded for
  production); Play Integrity against a fake Google (token and decode endpoints).
- The app gets the seam for native attestation, a no-op until the accounts exist.
- `scripts/e2e.sh` grows to the journeys phases 6 and 7 added.

## Results

- `internal/platform/challenge`: 32-byte challenges in Valkey, 5 minutes, consumed atomically (`GETDEL`); concurrent
  uses succeed once. `POST /v1/auth/attestation/challenge` (public, rate-limited, idempotent).
- `internal/identity/attest`: `AppAttest` (chain to Apple's root, certified nonce, app id, counter 0, aaguid, key id),
  `PlayIntegrity` + `GoogleDecoder` (service account OAuth, decode, package, challenge or request hash, freshness,
  `PLAY_RECOGNIZED`, `MEETS_DEVICE_INTEGRITY`, signing certificate), `Router` by platform. Each forgery in the tests is
  refused by its own check; Apple's root fingerprint is pinned.
- Identity: the verifier gets the consumed challenge (never the fingerprint); no challenge, no check; `off` never
  calls the verifier. API: `ATTESTATION_MODE` `refuse_failed` / `require` applied before plays, top-ups and
  withdrawals (`403 attestation-required`); the web is never refused for it. Config refuses the strict modes without
  both verifiers.
- App: `DeviceAttestation` (no-op `NoDeviceAttestation`); sign-in asks for a challenge and attaches the token only
  when a native attestor exists; a failed challenge request never blocks sign-in (the server decides).
- `scripts/e2e.sh` (`make e2e`, CI): sign-up, KYC, top-up, paybill, a play (day and wallet agree, phone proved), a
  withdrawal followed until paid, an attestation challenge, and the back office with real staff tokens from the local
  realm (roles seen, a reconciliation run, a wrong role refused, a player token refused).

## Blocked on the owner

| Item | Needs |
|---|---|
| Native App Attest (iOS) | Apple developer account, team id, App Attest capability; then `APP_ATTEST_APP_ID` |
| Native Play Integrity (Android) | Play Console app linked to a Cloud project, the Play Integrity API, a service account; then `PLAY_INTEGRITY_PACKAGE`, `PLAY_INTEGRITY_SERVICE_ACCOUNT`, `PLAY_INTEGRITY_CERT_DIGESTS` |
| `ATTESTATION_MODE=refuse_failed`, then `require` | Both of the above, released apps, a period of `record` to see real verdicts |
| Google / Apple sign-in | OAuth client ids (`GOOGLE_CLIENT_ID`, `APPLE_CLIENT_ID`); the API already verifies their ID tokens |
| Push notifications | A Firebase project (FCM) and APNs key; not built |
| Store release | Licensing and store approval (`docs/checkpoints/phase-6.md`, release blockers) |
