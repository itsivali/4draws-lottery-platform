# 0018. Staff sign in with Google or Apple; members and roles are kept by the platform

Status: accepted (supersedes the identity part of ADR 0013); roles and the sign-in providers superseded by 0019 (two roles; email and password, Google and Apple optional)
Date: 2026-10-26

## Context
ADR 0013 made Keycloak the staff identity: staff signed in to a staff realm and their roles were realm roles. No
Keycloak runs outside a laptop, so the back office could not be used online. The owner decided (2026-10-26) that the
back office becomes a full web app where staff sign in with Google or Apple, that roles are managed in the back
office, and that itsivali@gmail.com is the first admin.

## Decision
- **Sign-in:** the back office gets an ID token from Google Identity Services or Sign in with Apple JS, for its own
  web client (`STAFF_GOOGLE_CLIENT_ID`, `STAFF_APPLE_CLIENT_ID`), bound to a fresh nonce, and exchanges it at
  `POST /v1/staff/session` (signature, issuer, audience, expiry and nonce checked against the provider's keys).
- **Members:** `staff.members` (email, name, roles, active). An admin invites a member by the email address of their
  Google or Apple account. The first sign-in with a token whose email is verified and matches an active member binds
  that provider identity (`staff.identities`, append-only); later sign-ins use the binding, so Apple's one-time email
  and changed addresses do not matter. Nobody signs in uninvited.
- **Sessions:** the API's own opaque tokens (`stf_…`, only the SHA-256 stored), 12 hours at most and ended after 30
  idle minutes or on sign-out. Roles and the active flag are read on every request, so a role change or a disabled
  member applies at once (disabling also ends their sessions).
- **Roles:** the eight of ADR 0013 plus `admin` (invites, assigns roles, disables) and `finance` (the financial
  dashboard). Nobody changes their own roles or disables themselves, and the last admin cannot be removed. Every
  change is audited (`staff.invite`, `staff.roles`, `staff.disable`, `staff.enable`, `staff.sign_in`).
- **The first admin** is `STAFF_BOOTSTRAP_ADMIN` (itsivali@gmail.com on Render): made an admin at start-up while
  there is no active admin; it does nothing once one exists.
- **Locally**, the compose Keycloak realm stays as a stand-in provider (`dev`, accepted only when
  `STAFF_DEV_OIDC_*` is set, which no deployment does; its password grant has no nonce) with `ada` as the bootstrap
  admin, so development and the e2e journey need no Google or Apple account.
- Staff still appear as `staff:<member id>`; the two-person rules and `lotteryctl -operator` are unchanged.

## Consequences
- Two-factor authentication is the Google or Apple account's: the platform cannot require it. Staff should use
  accounts with 2-step verification on (a Google Workspace domain can enforce it).
- Apple's "Hide My Email" gives a relay address: invite that address, or the member shares their real one.
- Setting up the clients (Google Cloud OAuth web client with the back office's origin; Apple Services ID with its
  domain and return URL) is the owner's step (`docs/runbooks/back-office.md`).
- The Keycloak realm roles and the realm's API audience no longer decide anything.
