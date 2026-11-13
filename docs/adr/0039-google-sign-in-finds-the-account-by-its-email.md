# 0039. Google sign-in finds the account by its email address

Status: accepted (narrowed by ADR 0040: only an address the account has confirmed is matched)
Date: 2026-11-12

## Context

ADR 0020 let a Google or Apple identity sign in only once it was linked to an account; an unlinked one was offered a
new account. The owner signed in with Google using the address already on their phone-and-password account and was
asked to create an account (2026-11-12). They asked that the system look for the address first.

## Decision

**A Google identity linked to no account signs in the account whose email address it carries, and is linked to that
account from then on.** Only when no account has the address is the player offered a new one.

- **Only an address Google is the authority for:** the ID token says `email_verified`, and the address is a
  `gmail.com` one or the token carries a Workspace domain (`hd`). Google lets an account be opened on any address;
  for those it vouches for less, and they are not matched.
- The account must be active, as for any sign-in.
- The link is written once (`identity.auth_identities`) and audited as `sso.linked_by_email`. Later sign-ins find the
  link and do not look at the address.
- Apple is not matched: its addresses may be private relays, and it is switched off.

## Why this is safe enough

The address on an account is whatever was typed at sign-up; the platform does not verify it. That already means
**whoever owns the address can take the account**, by asking for a password-reset link (ADR 0021). Signing them in
with Google gives the same person the same account by another door, and nothing to anyone else.

The other direction: someone could open an account with another person's address and wait for that person to sign in
with Google and land in it. They would find an account with a stranger's name, phone number and identity on it.
Top-up prompts go to the account's own M-Pesa number and payouts go only there, after an identity check, so there is
no money to be drawn out of the newcomer this way. It is still a reason to verify email addresses at sign-up, which
is not built.

## Consequences

- An account can hold more than one Google identity (one linked earlier from Account, one found by address).
- A player whose account has no email address, or a different one, is still offered a new account; they can instead
  sign in with their number and password and link Google from Account → Sign-in methods.
