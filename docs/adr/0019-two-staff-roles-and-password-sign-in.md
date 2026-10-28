# 0019. Two back-office roles (admin, staff); staff sign in with email and password, Google and Apple optional

Status: accepted (supersedes the roles of ADR 0018 and 0013; keeps their sessions and provider binding)
Date: 2026-10-28

## Context
ADR 0018 gave the back office ten fine-grained roles and only Google or Apple sign-in. On 2026-10-28 the owner asked
for role-based access with two roles, Administrator and Staff, for a sign-in with a username and password with Google
or Apple as optional extras, and for the back office to show how the web and mobile apps are used.

## Decision
- **Roles:** `admin` and `staff`. A staff member may use every operational queue (identity review, payments, draws,
  payouts, KRA tax, alerts, account closures) and the finance dashboard. An admin may do everything staff may, and
  also manages members (invite, roles, disable, password resets) and reads the **app metrics**. `staff.Member.Has`
  treats an admin as staff, so every route needs exactly one of the two. The two-person rules (two different officials
  sign and approve each draw; the person who remits a KRA batch is not the one who prepared it) compare people, not
  roles, and are unchanged. Migration 00031 keeps existing admins and turns every other member with a role into staff.
- **Sign-in:** the username is the member's email address. `POST /v1/staff/session` with `provider: password` checks
  the password against an argon2id hash (`internal/platform/password`, peppered from `AUTH_KEY`); an unknown address,
  a wrong password and a member without a password are refused alike after the same work, rate limited per address and
  per client. Google and Apple (`provider: google|apple`) stay exactly as in ADR 0018 and are offered only when their
  client ids are configured; the `/v1/staff` routes no longer need them to be open.
- **Passwords are set by admins and changed by members.** An admin gives a member a temporary password when inviting
  them (`POST /v1/staff/members`, `password`) or later (`POST /v1/staff/members/{id}/password`, which also ends the
  member's sessions), and tells it to them out of band. A member signed in with a temporary password may only change
  it (`PUT /v1/staff/password`), see `/me` and sign out (`password-change-required`, 403, everywhere else). Changing
  it ends the member's other sessions. Rules: 12 to 128 characters, nothing from the common-password list, never the
  member's email address; the current password must match. Every set, change and reset is audited; no password is.
- **The first admin** (`STAFF_BOOTSTRAP_ADMIN`) may be given `STAFF_BOOTSTRAP_PASSWORD`, a deployment secret that
  becomes their temporary password while they have none; once they have changed it, it is never touched again.
- **App metrics** (`GET /v1/staff/metrics`, admins): per Nairobi day and platform (web, android, ios, from the device
  a session was opened with): sign-ups (the platform of the player's first device), sign-ins, players seen (a token
  issued to them), plays and stakes, top-ups and withdrawals from the wallet's transactions, attributed to the
  platform the player used that day (else their latest device). Read from the platform's own tables; no analytics
  SDK, nothing leaves the platform.

## Consequences
- Finer permissions are gone: a staff member who may review identities may also approve payouts. The owner chose
  this; the audit log still names who did what, and two-person rules still need two people.
- Passwords add the usual duties: admins hand temporary passwords over safely (not by chat history), members keep
  theirs in a password manager, and a compromised account is disabled from the Staff page (which ends its sessions).
  The platform cannot require a second factor for password sign-in; Google or Apple accounts with 2-step verification
  remain the stronger way in and should be preferred where they are configured.
- `lotteryctl -operator` and the audit records (`staff:<member id>`) are unchanged.
- The back office (login page, Staff page, App metrics page) and the runbook follow in their own changes.
