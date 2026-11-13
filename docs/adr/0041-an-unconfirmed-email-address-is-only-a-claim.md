# 0041. An unconfirmed email address is only a claim

Status: accepted
Date: 2026-11-13

## Context

ADR 0040 made an email address a claim until its link is opened, but the address was still unique among accounts.
So anyone could type another person's address at sign-up, never confirm it, and stop that person from using their
own address: sign-up answered `email-registered`, and Google sign-in told them to confirm an account that was not
theirs. Only support could remove it.

## Decision

**Several accounts may claim one address. Only one may hold it confirmed, and confirming it takes it from the rest.**

- **Uniqueness** is now on confirmed addresses only (`users_email_confirmed_key`). Sign-up and a change of address
  are refused (`email-registered`) only when another account has *confirmed* the address.
- **Confirming** (the emailed link, or Google vouching for the address) removes the address from every other
  account that only claimed it. Each removal is audited as `identity.email_claim_released`. Links sent to those
  accounts confirm nothing afterwards.
- **Google sign-in** with an address that is only claimed still answers `sso-email-unverified`, since the account
  may be the player's own. The app now also offers "Create a new account instead": the new account takes the
  address, confirmed, and the claim is released.
- **Password resets** for an address go to the account that confirmed it, or, when none has, to the oldest claim.
- **The back office** shows "Email not confirmed" on a player whose address is only a claim.

## Consequences

- An account that only claimed an address can lose it without notice when the owner confirms. It keeps its phone
  number and password; it can add another address.
- While an address is only claimed by several accounts, a reset link for it goes to the oldest of them. The reader
  of the mailbox is its owner either way.
- Rolling the migration back keeps the confirmed holder of each address (or the oldest claim) and clears the rest.
