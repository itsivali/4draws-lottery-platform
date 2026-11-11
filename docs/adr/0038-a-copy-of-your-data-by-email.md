# 0038. A copy of your data, by email

Status: accepted
Date: 2026-11-11

## Context

The Data Protection Act 2019 gives a person the right to see the data held about them. The privacy statement told
players to write to an email address for it, and staff would have had to assemble the answer by hand. The owner asked
that a player's request for their data be answered to their email address (2026-11-11).

## Decision

**A signed-in player asks in the app (Account → Get a copy of your data) and the server emails the file to the
address on the account.** `POST /v1/account/data-export`.

- **Two attachments, as Google and Microsoft give people their data: one to read, one for machines** (owner,
  2026-11-11: "properly formatted, easy to read"). `4draws-data-<date>.html` is a self-contained page that opens in
  any browser from the email and prints cleanly: who the player is, a contents list, and a headed table per kind of
  record, with dates in East Africa Time, amounts as "KES 1,250" and codes in words. It has no script and fetches
  nothing. `4draws-data-<date>.json` is the same data for another service or program (the right to portability).
  Unlike theirs it is attached, not linked: the owner asked for it to arrive in the email itself.
- **The data** is built in one read: the account, sign-in links, devices and sign-ins, deposit limits, breaks
  and exclusions, top-ups with their M-Pesa receipts, wallet movements, plays with their slots, draw entries, prizes,
  withdrawals and payouts, and the times earlier copies were sent. Each list names its columns, so nothing internal
  (token hashes, raw callbacks, idempotency keys, staff ids) can ride along.
- **Email is not a safe place for an ID number**, so the file carries its last three digits only, as the app itself
  shows it, and never the password hash. The file says what it leaves out and where to write for it.
- **To the account's own address only.** There is no parameter for another one. An account without an address is
  told to add one (`no-email`, 409).
- **Once in 24 hours** per player (`data-export-too-soon`, 429 with Retry-After), on top of the usual rate limit.
- **Recorded, not kept:** `identity.data_exports` (append-only) holds when, the size and the SHA-256 of each file,
  never its contents; the audit log gets `identity.data_exported` as the player's own act.
- **Sent inside the transaction that records it**, so a refused message records nothing and can be asked for again.
- **No mail server, no feature:** without `SMTP_HOST` the route answers `data-export-unavailable` (503) and points to
  the privacy address, instead of logging the file or pretending.

## Consequences

- The copy must be asked for **before** an account is closed: closing erases the email address (ADR 0037).
- A player whose email account is compromised exposes this file to whoever holds it. The masked ID number and the
  absence of credentials limit what that gives away; the email also tells the player what to do if they did not ask.
- Production needs working SMTP settings and a sender on the service's domain for this to do anything.
- Some mail services warn about HTML attachments or show them only after a download. A PDF would avoid that and
  needs a PDF library the project does not have; the page prints to PDF from any browser.
