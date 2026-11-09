# Runbook: payouts (withdrawals, and prizes from phase 5 slice 3)

Money out goes through M-Pesa B2C (`docs/checkpoints/phase-5.md`). A withdrawal debits the wallet when it is
requested; the worker sends it once and confirms the outcome with Daraja's Transaction Status API. The money comes
back to the wallet only when the failure is confirmed. Sandbox only (rule 13).

## Turning payouts on (sandbox)

Set `DARAJA_B2C_SHORTCODE` on Render (the sandbox test shortcode, `600992`; it shares `DARAJA_INITIATOR_NAME` and
`DARAJA_SECURITY_CREDENTIAL` with the paybill) and deploy. Daraja posts to
`<api>/hooks/b2c/<sha256 of DARAJA_CALLBACK_TOKEN>/result|timeout|status-result|status-timeout`; nothing needs
registering. Without the shortcode `POST /v1/payouts/withdrawals` answers 503.

## States

| State | Meaning | Who acts |
|---|---|---|
| held | waits for **two different** staff approvals (ADR 0032): above KES 100,000, or the account could not be paid when its turn came (reason `payee_ineligible: …`, ADR 0030): waits for an operator | operator: approve or reject |
| pending | the send job is queued | worker |
| sending | committed just before the B2C request | worker; if stuck it becomes timed_out, **never sent again** |
| sent | Daraja accepted the request | worker waits for the result, then asks Transaction Status |
| timed_out | the request's outcome is unknown | worker asks Transaction Status |
| result_received | Daraja reported an outcome (a claim) | worker confirms it |
| succeeded | confirmed paid; the receipt is recorded | — |
| failed | confirmed not paid; the money is back in the wallet | — |
| manual | the outcome could not be confirmed | operator, from the M-Pesa statement |

## Operator actions

```sh
go run ./cmd/lotteryctl payouts list -state held
go run ./cmd/lotteryctl payouts approve -payout <id> -operator <your back-office email>      # needs the api's B2C configuration; a second person approves too
go run ./cmd/lotteryctl payouts reject -payout <id> -reason "<why>" -operator <your back-office email>
go run ./cmd/lotteryctl payouts list -state manual
```

A payout held with `payee_ineligible: …` was not sent because the account cannot be paid now: it is suspended or
closed (`account not active`), its identity is no longer verified, its phone is not proved, or another number was
proved (`number changed`). Approving is refused (409 `payee-ineligible`) until the account may be paid again. If it
will not be (a closed account, a confirmed fraud), reject it with the reason: a withdrawal's money goes back to the
wallet, a prize goes back to its pool, still owed.

For a payout in `manual`, find it on the B2C account statement (the originator conversation id is the payout's
`originator_conversation_id`; the amount is `net_kes`, to the payout's number):

Settling takes two people (ADR 0032): the first records what the statement shows (a proposal, shown on the payout), a
second checks the statement and makes exactly the same decision, which applies it. A different decision replaces the
proposal; the proposer cannot confirm their own.

- found → `payouts settle-paid -payout <id> -receipt <receipt> -operator <your back-office email>`
- not there (and the payout is more than a day old) → `payouts settle-failed -payout <id> -reason "<why>" -operator <your back-office email>`;
  the amount returns to the wallet.

Never settle a payout as failed while it might still complete: that would refund money that left.

## Refund to source (ADR 0031)

For a balance the account cannot withdraw: suspended, identity refused or never verified, phone not proved. An admin
opens the player in the back office → **Refund to source**, with the reason. The whole balance leaves the wallet at once
as a withdrawal (the withdrawal tax rule in force applies) to the number of the account's latest credited top-up, and
waits in *Payouts → Held* marked *Refund to source*. **Someone else** approves it there; the admin who asked cannot
(409 `two-person-rule`). Rejecting it puts the money back in the wallet.

It is refused while the account can withdraw for itself (`payable`), owes money from a reversed top-up (`in-debt`),
has an empty wallet, or has no credited top-up to send to (`no-source`; settle such a case outside the platform with
counsel). Once the refund succeeds the account can be closed.

Questions for counsel, open: whether a refund to an underage account must also return the stakes it played, and
whether the withdrawal tax applies to a refund.

## Reasons a payout lands in manual

`unconfirmed` (no confirmation after the status requests), `success_not_confirmed` (Daraja said paid, Transaction
Status never confirmed it: a possible forged or early result), `status_amount_mismatch`, `credited_elsewhere`,
`receipt_reused`, `status_request_rejected` (usually the initiator or credential).

## Prizes

Every 5 minutes the worker creates a payout for each prize of an approved draw that has never had one: the winnings
tax (placeholder 20%) is withheld, the prize leaves its `PRIZE_POOL_*` for B2C (`PromotionPayment`), and the payment
follows the same states as a withdrawal (above KES 100,000 it is held for approval). Once every prize of a draw is
confirmed paid, the draw becomes `paid`.

- **A prize first repays the winner's debt** from a reversed top-up (owner's decision, 2026-10-25). The tax is withheld
  on the whole prize, the debt is taken from what is left, and only the rest is sent (`debt_repaid_kes` on the payout,
  in the audit log and in the KRA event). A prize the debt absorbs entirely sends nothing: it is recorded on the
  `offset` route, `succeeded` at once, with no receipt. If a partly offset prize's payment fails, the debt it repaid is
  owed again with the prize; a retry repays whatever is owed then.
- A winner who cannot be paid now (suspended, or no longer verified) is skipped; the prize stays owed. Fix the account,
  and the next run pays it.
- A prize whose payout **failed** (confirmed not paid) is back in its pool and is **not** retried automatically (a number
  that cannot receive M-Pesa would loop). Check the winner's number, then:
  `go run ./cmd/lotteryctl payouts retry-prize -result <draw result id> -operator <your back-office email>`.

## Tax withheld (KRA)

Each paid payout with tax withheld adds one `withholding_tax` event to the KRA feed. Events are remitted in numbered
batches, prepared and remitted by two different people: see [tax](tax.md).

## Payout freeze

While a failed reconciliation's alert is open, no payout is sent. They wait in `pending` and go out once a person
acknowledges the alert: see [alerts](alerts.md).

## Reconciliation

Daily (`go run ./cmd/lotteryctl payouts reconcile` runs it now; runs kept in `payouts.reconciliation_runs`): every
payout has its journal with exactly its net to `B2C_FLOAT` and its tax to `TAX_PAYABLE`; failed payouts, and only
they, have the reversal; prize payouts match their draw result; every paid payout with tax is in the KRA feed;
`B2C_FLOAT` and `TAX_PAYABLE` equal the net and tax of payouts that have not failed. The draw reconciliation checks each
pool against approved pools minus prizes paid out. A mismatch is a bug or tampering: stop approving payouts and
investigate.
