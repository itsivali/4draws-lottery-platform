# 0022. Identity is verified before the first withdrawal, not before the first top-up or play

Status: accepted (changes the eligibility split of ADR 0005; ADR 0009's manual review stays)
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
