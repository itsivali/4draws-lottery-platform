# 0040. Email addresses are confirmed by an emailed link; emails carry the brand

Status: accepted
Date: 2026-11-13

## Context

Players give an email address at sign-up (ADR 0021). It was never checked: a mistyped address, or someone else's,
stayed on the account. ADR 0039 then let Google sign a player in to the account that has their address, and named
the gap: someone could open an account with another person's address and wait for that person to arrive through
Google. The owner asked for email verification at sign-up (2026-11-13), and for a full email template with the logo.

## Decision

**An email address is a claim until the player opens a link sent to it.**

- **When the link is sent:** at sign-up, whenever the address changes, and again when the player asks (Account →
  Email address). The link opens `/verify-email?token=…` on the web app, works for 24 hours and once, and needs no
  session, because it is often opened on another device.
- **What is stored:** `identity.users.email_verified_at`, and one row per link in `identity.email_verifications`
  (the SHA-256 of the token, the address it was sent to, expiry, use). A link confirms only the address it was sent
  to: after a change of address, older links do nothing.
- **Sign-up does not wait for it.** The account is created and usable at once. If the email cannot be sent, the
  account still stands and the player can ask again.

**What an unconfirmed address may not do:**

| | Unconfirmed | Confirmed |
|---|---|---|
| Receive a password-reset link (ADR 0021) | Yes | Yes |
| Be found by Google sign-in (ADR 0039) | No: 409 `sso-email-unverified` | Yes |
| Receive a copy of the player's data (ADR 0038) | No: 409 `email-not-verified` | Yes |

Password resets stay open to unconfirmed addresses so that no existing player is locked out: none of the addresses
given before this change is confirmed.

**Google confirms what it vouches for.** An account created with Google takes the Google address, already confirmed,
unless another account has that address. A Google identity already linked to an account confirms the account's
address when it is the same one.

**Emails carry the brand.** `mail.Notice` renders one message twice: plain text, and a page with the logo on the
brand's indigo, a heading, one button, the link written out, facts, a safety note and a footer naming
4 DRAWS TICKET LTD. The logo rides inside the message (a `cid:` image), so nothing is fetched from the web when the
email is opened and it shows where remote images are blocked. Player emails (confirm address, reset password, copy
of data) and the staff password reset use it. Alert and review notices to staff stay plain text.

## Consequences

- **Existing accounts are unconfirmed.** Google sign-in by address (ADR 0039) stops working for them until the
  player signs in the other way and confirms. The app shows "Email not confirmed yet" on Account and offers the link
  again. No email is sent to existing players by the migration.
- An address someone typed and never confirmed still blocks another sign-up with it (`email-registered`). Support
  removes it on request. Letting unconfirmed addresses be claimed by their owner is not built.
- Closing an account erases the links and the confirmation with the address (ADR 0037, the function is replaced in
  migration 00058).
- The player's copy of their data shows when the address was confirmed.

## Alternatives considered

- **Block sign-in until confirmed.** Refused: the phone number, not the address, is the account's identity, and a
  lost or late email would stop a player at the door.
- **A six-digit code instead of a link.** A link is one tap and matches the reset flow already built.
- **Let Google sign in an unconfirmed account and reset its password.** It would hand the newcomer an account with
  someone else's phone number and identity on it.
