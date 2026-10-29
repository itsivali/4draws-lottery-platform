# 0020. Players sign in with Google or Apple; an identity without an account gives its phone number

Status: accepted (supersedes the "linking needs a verified ID and a proved phone" part of ADR 0005 for accounts created
this way; ADR 0005's sign-up, phone proof and password rules otherwise stand)
Date: 2026-10-29

## Context
ADR 0005 allowed a Google or Apple identity only as a link an already verified account could add, and the player app
never offered it. On 2026-10-29 the owner asked for Google and Apple on the player sign-in screen, with the player then
asked for a phone number. The platform still has no SMS and no email for players: the phone number is proved by the
M-Pesa confirmation of the first top-up, and that must stay the only proof.

## Decision
- **The sign-in screen offers Google and Apple** when the API lists them (`GET /v1/auth/providers`: the client ids the
  apps request ID tokens for; none configured, no buttons). Each provider's own sign-in runs on the device or in the
  browser; the app only ever receives an ID token bound to a nonce.
- **A token signs in or signs up.** `POST /v1/auth/sso/{provider}` signs in the account the identity is bound to. For an
  identity with no account it answers 409 `sso-not-linked`; the app then asks for the M-Pesa number and the 18+
  confirmation, and nothing else, and `POST /v1/auth/sso/{provider}/signup` (the same token, within its lifetime)
  creates the account: the identity bound (`identity.auth_identities`), no password, the number a **claim** until the
  first top-up proves it, under ADR 0005's rules (a number with a proved owner cannot be claimed, 409
  `phone-registered`; a proved owner excludes every other claim). Migration 00035 drops the constraint that a proved
  number implied a password. Audited as `user.created` (with the provider) and `sso.linked`.
- **Tokens are verified like the staff's** (ADR 0018): signature against the provider's keys, issuer, expiry, nonce, and
  an audience among the app's client ids: `GOOGLE_CLIENT_ID` (the web app's OAuth client, which the phone apps name as
  their server client) and `GOOGLE_IOS_CLIENT_ID`; `APPLE_CLIENT_ID` (the Services ID the web app and Android use) and
  `APPLE_BUNDLE_ID` (the iOS app, whose native tokens name it). Android has no native Sign in with Apple: the app opens
  Apple in the browser with `POST /auth/apple/android` as the return URL, which only bounces Apple's fields back into
  the app (`ANDROID_PACKAGE`); the token is then verified like any other.
- **Password sign-in skips accounts without a password**, costing the same time as a wrong password, so a Google account
  cannot be entered with a guessed password and a number's proved owner still excludes other claims. Staff may give such
  an account a temporary password from the Players page (ADR 0019), after which the number and password sign in as well.
- **Linking from inside the app** (`POST /v1/auth/sso/{provider}/link`) keeps ADR 0005's rule (verified ID, proved
  phone); there is still no screen for it.

## Consequences
- These players have no password to forget. A lost Google or Apple account is recovered by staff (temporary password).
- The player's email address is never requested beyond what the provider's own screen shows, and never stored: the
  binding is the provider's stable subject. Apple's "Hide My Email" changes nothing.
- Google's OAuth consent screen must be published (in testing it admits only listed testers); Sign in with Apple needs
  the paid Apple Developer Program, and the iOS app the capability (`Runner.entitlements`). The owner's steps are in
  `docs/runbooks/player-sign-in.md`.
- A sign-up through a provider still cannot play or top up before identity verification and the phone proof; the
  eligibility rules are unchanged.
