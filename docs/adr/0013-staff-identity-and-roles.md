# 0013. Staff sign in with Keycloak; every back-office action needs a role and runs with the worker's database role

Status: accepted
Date: 2026-10-26

## Context

Operators have run every staff action with `lotteryctl` and a free-text `-operator NAME`. Nothing proved who that
was, and the two-person rules (two officials sign and approve a draw; one person prepares a KRA batch and another
records its remittance) compared names typed by whoever ran the command. Phase 6 brings a back office, which needs a
real staff identity and permissions.

The player API's database role (`app_api`) may not update draws or payouts (migration 00008), on purpose: a bug or
compromise in the player API must not be able to change a draw or release money.

## Decision

- **Staff are a separate identity, issued by a staff realm in Keycloak** (local: `deploy/keycloak`, imported by
  `make dev`). The back office signs in with OIDC Authorization Code + PKCE; the API accepts the realm's access tokens
  (RS256, issuer, audience `lottery-backoffice-api`, expiry; `identity.OIDCVerifier.VerifyAccessToken`). Player tokens
  never work on staff routes and staff tokens never work on player routes.
- **Roles are realm roles**: `kyc_reviewer`, `payments_operator`, `draw_official`, `payout_operator`, `tax_officer`,
  `alerts_operator`, `support`, `auditor` (`internal/staff`). Every `/v1/staff` route requires one (the auditor reads
  and runs reconciliations, changes nothing). Roles unknown to the platform are ignored; a token with none is refused.
- **Staff appear as `staff:<Keycloak subject>`** in audit records and in the services' operator fields, so the
  two-person rules compare real, distinct people. `lotteryctl` keeps working (`operator:<name>`) for emergencies.
- **The back office acts through services on `STAFF_DATABASE_URL`**, connected with the worker's role. Without it
  (single-role mode, as on Render's free plan today) they share the API's pool.
- Configuration is all-or-nothing (`STAFF_OIDC_ISSUER`, `STAFF_OIDC_AUDIENCE`, `STAFF_OIDC_JWKS_URL`); without it the
  staff routes answer 503.

## Consequences

- Production needs a Keycloak (or another OIDC provider issuing the same claims), with MFA required for staff, and
  someone who assigns roles. Owner decision: where it runs and who administers it.
- Role mode needs the worker role's login for the back office (phase 8: login roles).
- The draw entry list (which ties entries to players) stays out of the back office: it is for the regulator
  (`lotteryctl draws entries`).
