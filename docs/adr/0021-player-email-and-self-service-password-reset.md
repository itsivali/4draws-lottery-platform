# 0021. Players give an email address at sign-up and reset a forgotten password themselves

Status: accepted (supersedes ADR 0005's "no password reset" and the staff-only reset of ADR 0019 as the only way)
Date: 2026-10-29

## Context
Players had no channel the platform could reach them on: no SMS (cost, ADR 0005), no email. A forgotten password
could only be replaced by staff from the back office (a temporary password, ADR 0019, #84). On 2026-10-29 the owner
asked for an email address at sign-up so that players reset their own password.

## Decision
- **Sign-up asks for an email address** (`identity.users.email`, lower-cased, unique among accounts; migration 00037).
  It is not verified: it is a way to reach the player, not proof of anything, and the phone number remains the
  identity that the first top-up proves. Accounts from before have none until the player adds one from the account
  screen (`PUT /v1/account/email`, with the password in hand when the account has one). A Google or Apple sign-up
  (ADR 0020) asks for none; such an account may add one.
- **Forgotten passwords are reset through an emailed link**, exactly as staff's are (ADR 0019): `POST
  /v1/auth/password/forgot` always answers 202 and emails a link to the web app's `/#/reset-password?token=…` when the
  address has an active account; the link works for 30 minutes and once (`identity.password_resets`, the token's
  SHA-256 only); `POST /v1/auth/password/reset` sets the password, ends every session of the account and spends every
  open link. Rate limited per address and per client. Audited (`auth.password_reset_requested`,
  `auth.password_reset_by_email`, `identity.email_changed`); the address itself is never in the audit log.
- **The link needs the web app's address** (`WEB_APP_URL`) and the platform's SMTP (the same Resend account as the
  staff notices). Without `WEB_APP_URL` the endpoint answers 503 and the app tells the player to ask for a temporary
  password. Staff's temporary password stays as the fallback and for players without an address.
- The back office's Players page shows the address and searches by it.

## Consequences
- An address can be claimed by the wrong person (no verification): that only blocks the real owner from signing up
  with it until staff sort it out, since a reset link reaches the address, not the claimant, and the account it resets
  is the claimant's own. Verification can be added later (a link at sign-up) without changing the model.
- Players' email addresses are personal data: they live in `identity.users` with the phone number, under the same
  access rules, and are shown to staff only on the Players page (audited as a view).
- Older app builds that send no address cannot sign up (400 `invalid-email`); the web app updates itself, and no phone
  build has been distributed.
