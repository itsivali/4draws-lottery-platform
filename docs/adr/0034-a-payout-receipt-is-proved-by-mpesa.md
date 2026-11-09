# 0034. A payout's receipt is proved by M-Pesa, not by staff

Status: accepted (amends 0032 for settling as paid)
Date: 2026-11-09

## Context
A payout lands in `manual` when its outcome could not be confirmed: Daraja's answers were lost, contradictory, or
Transaction Status could not find it. Staff then look at the B2C statement. ADR 0032 had "paid" settled by two
people agreeing on the receipt they read there.

The owner decided on 2026-11-09 that a receipt is a valid way to settle a payout only when M-Pesa itself verifies it.
Two people agreeing is not proof that money left; M-Pesa's own records are.

## Decision
- **Recording a receipt does not settle anything.** `Settle(paid, receipt)` records the receipt found on the statement
  as a check (`payouts.receipt_checks`, migration 00052) and asks M-Pesa about it with Transaction Status by that
  receipt, from our B2C shortcode. The answer comes to the payout status hook like any other, and the verify job
  decides the check.
  - **Confirmed** settles the payout as paid under that receipt, with the claimant as `settled_by`. M-Pesa must
    report: completed, the same receipt, the payout's net amount, and the payout's number (when it shows one).
  - **Refuted** leaves the payout in `manual`, with what M-Pesa said. This covers anything else M-Pesa reports: an
    unknown receipt, another amount, another number, not completed.
  - **No answer** (a queue timeout, or no answer within 10 minutes) leaves it in `manual` too; staff may record a
    receipt again.
- **Rules that hold throughout:**
  - A malformed receipt or one that already settled another payout is refused up front.
  - While M-Pesa is being asked, nothing else is recorded on the payout: no other receipt, and no "not paid".
  - A check is decided once; a trigger forbids rewriting it.
- **"Not paid" keeps ADR 0032's two people.** M-Pesa cannot prove that a payment did not happen, so the second
  person's confirmation stays the control.

## Consequences
- One person can settle a paid payout, because M-Pesa is the second party.
- Settling as paid needs Daraja's B2C configuration, so `lotteryctl payouts settle-paid` needs it like `approve` does.
  A payout whose receipt M-Pesa will not confirm stays in `manual` until M-Pesa does, or two people settle it as not
  paid.
- **What is not checked yet.** The paying shortcode in M-Pesa's answer is not compared with ours. The question is asked
  from our shortcode, and an answer about another organisation's payment has not been seen. Comparing them is a small
  later hardening if Safaricom documents the field for B2C.
