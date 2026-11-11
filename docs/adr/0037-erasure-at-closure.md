# 0037. Erasure at closure

Status: accepted
Date: 2026-11-11

## Context

Closing an account (ADR 0015's first half, built in migration 00027) only blocked it: the status became `closed`
and its sessions were revoked. Everything else stayed: the password hash, the sessions and devices, the links to
Google or Apple, the photo, the email address. The public deletion page (`/delete-account/`, 2026-11-11) says these
are deleted when an account closes, and the Data Protection Act's storage-limitation principle says the same of data
no longer needed. The owner asked for it to be built (2026-11-11).

ADR 0015 is still proposed: the retention periods, and what happens when they end, wait for counsel.

## Decision

**When staff close an account, what the law does not require the operator to keep is erased in the same
transaction.** An account is never closed with that data still there, and never erased without being closed.

Erased:

| Data | Where |
|---|---|
| Password hash, "must change password" flag | `identity.users` |
| Email address | `identity.users.email` |
| Profile photo address | `identity.users.photo_url` |
| Sessions and refresh tokens | `identity.sessions`, `identity.refresh_tokens` |
| Devices (fingerprint hash, platform, attestation verdict) | `identity.devices` |
| Links to Google or Apple | `identity.auth_identities` |
| Password-reset links | `identity.password_resets` |

Kept, because gambling, anti-money-laundering and tax law require records of who played and where the money went:
the account row with its phone number, national-ID hash, status and dates; identity checks and declarations (`kyc.*`,
encrypted); payments, plays, tickets, draws, prizes, payouts, the wallet and the ledger; tax; limits, breaks and
self-exclusions; monitoring cases; the status history; the audit log.

How:

- One database function, `identity.erase_closed_account(user, at)`, does it. It is `SECURITY DEFINER`, owned by the
  migrator, and **refuses any account whose status is not `closed`**. The application roles get `EXECUTE` on it and
  no `DELETE` on those tables, so nothing else in the application can delete a session or a device.
- `identity.Service.CloseAccount` calls it after marking the account closed, in the closing transaction.
- `identity.users.erased_at` records when. Calling the function again removes nothing more and keeps the first time.
- The audit log gets `identity.account_erased` with what was removed as counts and yes/no (never the values), beside
  `identity.account_closed`.
- Migration 00056 erases accounts that were closed before this existed.

## Consequences

- A closed account's phone number stays on its row, so that number cannot open a new account. That was already so;
  whether a returning player may reuse a number is a question for ADR 0015.
- The "same device on several accounts" monitoring rule (ADR 0033) no longer sees a closed account's devices. A
  person who closes an account and opens another on the same phone is still tied to the first by their ID number.
- App metrics that count sign-ins by platform lose a closed account's past sessions.
- **Not built:** deleting or anonymising the kept records when their retention period ends. Nothing can fall due for
  years, and the periods are counsel's to set (ADR 0015).
- A player who wants a copy of their data must ask before the account is closed: afterwards there is no email
  address to send it to.
