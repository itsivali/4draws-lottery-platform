# 0022. Identity is verified before the first withdrawal, not before the first top-up or play

Status: accepted (changes the eligibility split of ADR 0005; ADR 0009's manual review stays); amended by 0029: in the live operating mode verified ID comes before money in, and sign-up takes a date of birth
Date: 2026-10-29

## Context
ADR 0005 required a verified national ID before a top-up and a play, and ADR 0009 made that verification a manual
review by staff. With a spin at KES 1 (migration 00036) every new player would wait for a person before their first
play; the owner decided (2026-10-29) that the review is too much work to stand in front of play and asked for it to
move to the point where money leaves: the first withdrawal.

## Decision
- **Top-ups and plays need an active account and (for plays) a proved phone number**, nothing else. The first top-up
  still proves the number (ADR 0005) and remains the only proof the platform accepts.
- **Withdrawals and prize payouts need a verified national ID** (`AssertCanBePaid` is now the one place identity is
  checked, together with the proved phone). The age check is part of that verification: an under-18 can play but can
  never be paid, and the money stays in the wallet until the account is closed and refunded under the operator's
  procedure. The 18+ confirmation at sign-up remains a declaration.
- The app no longer blocks the Play and Top up screens on the ID; the Home screen suggests verification as the thing
  to do before withdrawing, after the number is confirmed. Paybill payments into an unverified account are credited
  (the `kyc_required` suspense reason is gone).
- **A new account gives its first name, second name and national ID number at sign-up** (owner, 2026-10-30;
  `kyc.declared_identities`, encrypted, migration 00041). They are shown back to the player and to staff, and the
  verification before the first withdrawal starts from them. **No date of birth is asked under manual review**: an
  M-Pesa line is registered to an adult against their ID, so reviewers check that the name matches the ID number (and
  the name M-Pesa shows for the number). A registry, if one is configured, still needs the date of birth.
- The review itself (ADR 0009, the back office's Identity review queue) is unchanged. Linking a Google or Apple
  identity to an existing account (ADR 0005) still requires a verified ID; signing up with one does not (ADR 0020).

## Consequences
- Money from unverified, possibly under-age or sanctioned players enters the wallets. Reviewers see it only at the
  first withdrawal; a refused identity leaves a balance that support must deal with by hand (close and refund). The
  regulator's view of play before identity verification is the owner's question, as with ADR 0009.
- The review queue is smaller (only players who want to withdraw) and later (a player can play for weeks before they
  appear in it), so the first withdrawal is slower than the first play. To take the person out of the loop, the
  gateway code kept by ADR 0009 (`KYC_API_URL`) can be pointed at a registry reseller (in Kenya, IPRS access through a
  licensed vendor such as Smile ID, priced per check): an exact match verifies at once, only mismatches reach the
  queue. That is a vendor contract and a per-check cost, the owner's decision.

## Addendum, 2026-10-31: names declared later
Accounts from before 2026-10-30 have no declaration, so the app showed their phone number where the name goes. The
owner asked for a way for them to fill the names in, and for the account screen to become a profile the player
manages. Decisions:
- **A declaration can be made later, once, until a submission exists.** `POST /v1/account/identity` takes the same
  first name, second name and national ID number as sign-up, with the same validation (`kyc.Service.DeclareLater`).
  It is refused once the account has a declaration (`identity-declared`) and once an identity submission exists
  (`kyc-state`): the name on the submission is the one shown and reviewed, and a declaration can never be edited, only
  checked. The row stays append-only (migration 00041); the act is audited as `identity.declared` with the source only.
- `GET /v1/account` says where the identity stands (`identity.status`: `none`, `declared`, `submitted`, `verified`)
  and gives the last three digits of the ID number, never the number, so the app can show it masked. Home asks an
  account with `none` to add its names until it does; the account screen's "Your details" holds the form, then the
  names and the masked number.
- With it, the account shows the **devices signed in** (`GET /v1/account/sessions`, open sessions only; `POST
  /v1/account/sessions/revoke-others` ends every other one, reason `logout`, audited `auth.sessions_revoked` with the
  count) and the **sign-in methods** (`GET /v1/account/sign-ins`: a password or not, and the Google or Apple identities
  linked, ADR 0020). Linking still happens from the sign-in screen and still needs a verified ID.
