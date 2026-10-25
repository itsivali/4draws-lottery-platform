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
| held | above KES 100,000: waits for an operator | operator: approve or reject |
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
go run ./cmd/lotteryctl payouts approve -payout <id> -operator <name>      # needs the api's B2C configuration
go run ./cmd/lotteryctl payouts reject -payout <id> -reason "<why>" -operator <name>
go run ./cmd/lotteryctl payouts list -state manual
```

For a payout in `manual`, find it on the B2C account statement (the originator conversation id is the payout's
`originator_conversation_id`; the amount is `net_kes`, to the payout's number):

- found → `payouts settle-paid -payout <id> -receipt <receipt> -operator <name>`
- not there (and the payout is more than a day old) → `payouts settle-failed -payout <id> -reason "<why>" -operator <name>`;
  the amount returns to the wallet.

Never settle a payout as failed while it might still complete: that would refund money that left.

## Reasons a payout lands in manual

`unconfirmed` (no confirmation after the status requests), `success_not_confirmed` (Daraja said paid, Transaction
Status never confirmed it: a possible forged or early result), `status_amount_mismatch`, `credited_elsewhere`,
`receipt_reused`, `status_request_rejected` (usually the initiator or credential).
