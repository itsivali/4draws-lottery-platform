# 0033. Transaction monitoring: rules raise cases; a case can hold a player's money

Status: accepted
Date: 2026-11-09

## Context
POCAMLA expects a reporting institution, which a licensed gaming operator is, to monitor transactions, look into
unusual ones and report suspicious ones to the Financial Reporting Centre (`docs/compliance/requirements-matrix.md`).
The platform had nothing of the kind (roadmap AML-01). A player could top up, barely play and withdraw, probe numbers
with failed top-ups, or run several accounts, and nobody would be told.

## Decision
- **Rules.** Five rules run hourly (the `aml.scan` job, or `lotteryctl aml scan`):

  | Rule | What it finds | Default threshold |
  |---|---|---|
  | `deposit_velocity` | Credited top-ups in the window above an amount or a count | 24h, more than KES 100,000 or more than 10 |
  | `in_out_little_play` | Withdrawals of at least a minimum while staking less than a share of what was deposited | 7 days, KES 5,000, under 20% played |
  | `failed_topups` | Failed, cancelled or expired top-ups | 24h, at least 5 |
  | `shared_id_number` | An ID number declared at sign-up by more than one account | 2 accounts |
  | `shared_device` | A device fingerprint used by several accounts | 3 accounts |

  Refunds to source (ADR 0031) are not counted as withdrawals.
- **Thresholds are the compliance owner's.** They are versioned settings (`compliance.aml_rule_settings`,
  append-only), changed with `lotteryctl aml set-rule` and audited. A rule can be turned off. Until a setting carries
  the compliance owner's approval reference, it is shown as *pending compliance-owner approval*, and every case it
  raises records that its thresholds were pending. The defaults above are placeholders.
- **Cases.** A rule opens one case per player and rule (`compliance.aml_cases`) with what it saw as evidence. It does
  not raise the same player again for that rule while a case is open, or within the rule's quiet period after one is
  closed (7 to 90 days by rule). Staff can open a case themselves. Everything done on a case is kept
  (`compliance.aml_case_events`, append-only). What a case is about never changes, and a closed case is final.
- **Holds.** Staff may hold a player's money while a case is open.
  - **What stops:** top-ups (and the prompts of recorded ones), paybill payments (refused at validation),
    withdrawals, refunds to source, and payouts. A payout already made goes back to held; a prize stays owed.
  - **Lifting:** someone other than the person who placed the hold lifts it, by releasing it or by closing the case.
  - **Play is not stopped.** The money stays inside the platform.
- **The player is not told why.** A refusal reads *This isn't available on your account right now* (403
  `account-unavailable`). Telling a player they are being looked into can be tipping off, itself an offence.
- **Back office.** The *Compliance* page lists open, held and closed cases with what the rule saw. From it staff hold,
  lift, note and close cases, and see the rules with their approval state.

## Consequences
- **A foundation, not compliance.** This does not claim to satisfy POCAMLA. The compliance owner decides the rules,
  the thresholds, the review process and what is reported to the FRC; reporting itself happens outside the platform.
  The outcome note records whether a case was reported.
- **False positives.** Families sharing a phone will raise `shared_device` cases; the rule can be turned off or its
  threshold raised.
- **Staff power.** A hold is one person's decision, because it stops money rather than moving it. Lifting a hold takes
  a second person, so a hold cannot be placed and lifted to hide a review.
