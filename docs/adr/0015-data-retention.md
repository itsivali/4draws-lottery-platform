# 0015. Data retention and closed accounts (proposed)

Status: proposed — needs the owner and counsel
Date: 2026-10-26

## Context

Players can ask for their account to be closed (ADR 0012); staff close it once nothing is owed either way. The
records of a regulated lottery must be kept: ledgers and payments (tax and anti-money-laundering), tickets, spins and
draw entries (the regulator), audit logs. Personal data beyond what those records need should not be kept longer
than necessary (Kenya Data Protection Act, 2019). The periods are legal decisions this repository cannot make.

## Proposal

- Keep, for the period the law sets (to confirm; commonly 7 years for financial records): the ledger, wallet
  transactions, payments, payouts, tickets, spins, draw entries, KRA records and the audit log. They are append-only
  already.
- At closing, end every session and stop all use (done). After a short grace period (to confirm, e.g. 90 days for
  disputes and reversals), remove what is not needed for those records: the password hash, devices and
  fingerprints, linked SSO identities, refresh tokens; and keep KYC details only in their encrypted form, readable only
  for a regulator's or court's request.
- At the end of the legal period, delete the KYC ciphertext's key for that player (crypto-shredding, ADR on keys) and
  replace the phone number with a stable pseudonym, so records stay consistent but no longer identify the person.
- A scheduled job does both steps and writes an audit event per account; nothing is deleted by hand.

## Open questions (owner, counsel)

The legal retention period for each record type; the grace period; whether the regulator needs the phone number in
clear for the whole period; how a player asks for a copy of their data (subject access).
