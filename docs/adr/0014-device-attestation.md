# 0014. Device attestation: server-issued challenges, server-side verifiers, a mode that grows to enforcement

Status: accepted
Date: 2026-10-26

## Context

Phase 1 stored a per-device attestation verdict and wrote `AssertDeviceTrusted`, but no verifier existed, the token
was bound to the device fingerprint (which a captured token could replay), and nothing called the check.

## Decision

- **Challenges:** `POST /v1/auth/attestation/challenge` issues a random challenge kept in Valkey for 5 minutes and
  consumed atomically on first use (`internal/platform/challenge`). The app attests to it and sends the token and the
  challenge with sign-up or sign-in. A token without a challenge this server issued and has just consumed is not
  checked at all.
- **Verifiers** (`internal/identity/attest`), server-side only:
  - App Attest: the attestation object's chain to Apple's App Attestation Root CA (embedded, fingerprint pinned by a
    test), the certified nonce = SHA-256(authData ‖ SHA-256(challenge)), our app id, counter 0, the production aaguid
    (development builds only when allowed), key id = SHA-256 of the certified key.
  - Play Integrity: Google decodes the token (service account, OAuth JWT bearer); we require our package, the
    challenge (classic nonce, or base64url SHA-256 as the standard request hash), a verdict under 5 minutes old,
    `PLAY_RECOGNIZED`, `MEETS_DEVICE_INTEGRITY`, and the signing certificate when configured.
- **`ATTESTATION_MODE`:** `off` (never verify), `record` (default: verify and store, block nothing),
  `refuse_failed` (devices that failed may not play, top up or withdraw), `require` (phones must also have passed).
  The web cannot be attested; whether web sessions may transact under the strict modes is `ATTESTATION_WEB` (below).
  The stricter modes refuse to start without both verifiers.
- A verifier error is "none" (unknown), never "passed".
- **The web cannot be attested, and anyone can claim to be "web"** (found in the phase 8 security review).
  `ATTESTATION_WEB=allow` (default) lets web sessions transact under the strict modes, so those modes stop only
  modified or unattested *phone* clients; `ATTESTATION_WEB=refuse` closes money routes to web sessions under the strict
  modes. A web app that moves money needs its own controls (a CAPTCHA or proof of work at sign-up, lower limits);
  none exists yet.

## Consequences

- Turning on `refuse_failed`/`require` needs: the App Attest capability and team id (Apple developer account); a Play
  Console app linked to a Cloud project with the Play Integrity API, and a service account; the native code in the
  app that produces tokens (`DeviceAttestation` in the app is the seam; it is a no-op until then). Owner actions.
- **Attestation is checked at sign-in only.** A passed verdict stays with the session; tokens copied from a genuine
  phone to a script keep it, and a genuine phone could attest challenges for other clients. The follow-up: store the
  App Attest key id per device and require assertions over the request on money routes; ask Play Integrity for a fresh
  token bound to the request hash on withdrawals; bind challenges to the requesting client.
