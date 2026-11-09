# 0024. Top-ups M-Pesa completed but the platform never credited are reconciled from the receipt

Date: 2026-10-31. Status: accepted; §1 (late callbacks) amended by 0028: a late callback is confirmed with M-Pesa before it credits.

## Context

On 2026-10-30 at 21:34 EAT a player (the owner) approved a KES 1 prompt and got the M-Pesa message, but Safaricom's
callback never reached the API, the sandbox throttled the status query, and after 15 minutes the top-up settled
`unconfirmed/paid_without_callback`. It was credited only when the owner typed the receipt from the message into the
back office. Nothing in the platform could do that on its own, and the player app said "Waiting for M-Pesa" (fixed in
#110) with no way to help.

What the platform can and cannot learn (ADR 0006, 0008):

- Daraja's STK status query says **whether** a prompt was paid, never the receipt or the amount. The sandbox's answer
  is not even reliable (it credited unpaid prompts before #106).
- Money is credited only against an M-Pesa receipt (`payments_credited_has_confirmation`): it is what the statement,
  reversals and the auditors reconcile on.
- A receipt exists in four places: Safaricom's callback (late or lost), the player's M-Pesa message, the business's
  M-Pesa statement, and Safaricom's Transaction Status API, which answers about a receipt it is given (amount, payer,
  the business paid, the state) when the paybill initiator is configured.

## Decision

A top-up that settled without being credited (`unconfirmed`, or `failed`/`cancelled`/`expired` when the player says
money left their M-Pesa) is reconciled from the receipt, by whichever of these comes first:

1. **A late callback** (`HandleCallback` on a final payment): a paid callback for a settled, uncredited top-up credits
   it at once (`source: late_callback`), with the usual receipt-uniqueness check. Before this, such a callback was
   ignored and the top-up stayed unconfirmed.
2. **The player's receipt** (`POST /v1/payments/{id}/receipt`, `ClaimReceipt`): the app asks for the receipt code
   from the M-Pesa message once a top-up settles uncredited. The receipt is a *claim* (`claimed_receipt`,
   `claim_status`), never a credit. When Transaction Status is configured the platform asks M-Pesa about it
   (`CheckClaim`, River job `payments.claim_check`, asked again by the sweep every 10 minutes while unanswered); the
   answer arrives on the paybill status hook and `settleClaim` credits the top-up only if M-Pesa says *Completed*, to
   our business (the prompt's shortcode or the paybill), for the amount the top-up asked for, and from the player's
   number when M-Pesa names it. Anything else leaves the claim `unverified` with M-Pesa's reason, for an operator. The
   player sees the claim's state (`receipt_claim` on the top-up) and may correct a mistyped code after a refusal.
3. **The M-Pesa statement** (`POST /v1/staff/payments/statement/match` then `/apply`): the operator uploads
   Safaricom's CSV export. Every paid row is matched by receipt (already credited; or the receipt a player claimed),
   else to exactly one prompt with the same amount, the same number (M-Pesa masks it: `254796***457`) and a time
   within 2 hours before to 20 minutes after the prompt. Several fits are `ambiguous`, none `unmatched`; credited
   prompts in the window missing from the statement are listed too. Matching credits nothing; the operator confirms
   the list and each credit stands alone. The import (file hash, counts, the report, the outcomes) is kept in
   `payments.statement_imports`, append-only, and audited.
4. **An operator**, one top-up at a time (`GET /v1/staff/topups/{id}` shows everything the platform knows):
   `recheck` credits an orphan or late callback and otherwise reports what M-Pesa's status query says now
   (`paid_without_receipt` means the receipt is still needed); `resolve` credits from a receipt, with a note; `dismiss`
   closes an unconfirmed top-up that is not on the statement as `failed/not_on_statement` and rejects any claim.

Every path credits through the same `creditPayment` (excise booked to the operator, debt recovered, phone proved,
`payment.topup.credited` with its `source`). A receipt credits one payment, ever. Nothing is credited on a player's or
a statement's word alone: the receipt must be confirmed by M-Pesa's callback, M-Pesa's Transaction Status answer, or
an operator looking at the statement.

## Consequences

- A paid prompt whose callback is lost is now credited without anyone typing, in most cases within a minute of the
  player entering the receipt, wherever Transaction Status answers. Where it does not (the sandbox may not know a
  prompt's receipt; the initiator may be unconfigured), the operator finds the receipt already filled in.
- The statement import replaces "find the payment on the statement by phone, amount and time" by hand, and records
  what was done with each file.
- Reconciliation gains `receipt_claims` in its attention list; the System page counts unconfirmed top-ups and open
  claims. Alerts on them follow (alerts expansion).
- The player app no longer tells anyone to keep waiting on a prompt that will not answer: it asks for the receipt.
- The Daraja mock issues ten-character receipts and answers Transaction Status about completed prompts, so the whole
  path is tested without Safaricom.
