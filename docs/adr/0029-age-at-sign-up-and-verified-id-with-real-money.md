# 0029. Age is declared at sign-up and checked by the server; with real money, verified ID comes before money in

Status: accepted (amends 0022)
Date: 2026-11-07

## Context
Only the app's "I am 18 or older" checkbox stood between a child and an account. The server took no date of birth at
sign-up. Under manual review (owner, 2026-10-30) the identity submission could also leave the date out and still be
stored as `is_adult = true`. ADR 0022 checks the national ID only before the first withdrawal, so money could come in
and be staked with no age evidence at all.

LN 112/2026 reg 82 asks a licensed operator to verify identity, age and location before a deposit
(`docs/compliance/requirements-matrix.md`). The owner decided on 2026-11-07:

- a date of birth and an 18+ declaration at sign-up, checked by the server, in every mode;
- a verified ID before money comes in, but only when the service runs on real money.

## Decision
- **Sign-up needs a date of birth and the declaration.** This applies to the password sign-up and to the Google/Apple
  sign-up alike. The request carries `date_of_birth` (YYYY-MM-DD) and `adult_declaration: true`. The server refuses
  before the account exists:
  - 400 `age-required` when either is missing;
  - 400 `invalid-date-of-birth` for a malformed or future date;
  - 403 `underage` when the player is under 18 on today's date in Nairobi.
- **What is stored.** The date and the declaration go to `kyc.age_declarations` in the transaction that creates the
  account (migration 00047). The date is encrypted like the identity submission's, and the row is append-only.
- **Identity verification always needs a date of birth.** `kyc.Submit` uses the one the player declared at sign-up when
  none is given. The manual-review exception of 2026-10-30 is withdrawn: under 18 is refused, and accounts from before
  this ADR must give a date.
- **With real money, verified ID comes before money in.** When `OPERATING_MODE=live` (ADR 0027), the app calls
  `identity.RequireVerifiedIDFirst`. `AssertCanTopUp`, `CanTopUp` (paybill validation) and `AssertCanPlay` then refuse
  an account whose national ID is not verified (`kyc-required`). In `sandbox` ADR 0022 stands: the ID is checked before
  the first withdrawal.
- **The apps ask for the date.** Sign-up asks for the date as on the ID (DD/MM/YYYY) under the ID number, refuses
  under 18 on the field, and sends both fields.

## Consequences
- **Older app builds.** An app built before this change cannot create accounts until it is updated: the server now
  refuses sign-ups without a date of birth. The web app ships with the API; mobile builds must be updated before this
  is deployed to players (none are in the stores yet).
- **Accounts created before this ADR.** They have no age declaration. They give a date of birth when they verify their
  ID, and with real money they cannot top up or play until they have.
- **What the declaration proves.** It is the player's statement, not proof of age. Proof is the verified ID, which
  real-money operation now requires before any money moves.
- **Still to do in S3** (the first three are done in S3b, ADR 0030):
  - re-check eligibility when a payout is sent or approved, and when the STK push job runs;
  - an account status history;
  - self-exclusion revoking sessions;
  - a way out for balances that cannot be paid;
  - a location signal.
