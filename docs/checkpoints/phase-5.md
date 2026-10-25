# Phase 5 — Payouts: checkpoint

Status: IN PROGRESS (started 2026-10-25, on the owner's instruction to build up to the web app).

## Decisions taken without the owner (placeholders, marked in code and below)

The owner asked for phase 5 to be built before these were settled; each is a placeholder until the owner (and a tax
adviser) confirms it, and nothing here reaches real money: Daraja stays in the sandbox (rule 13).
- **Withholding tax on winnings:** `winnings_wht` 20% of the gross prize, `half_up`, withheld when the prize is paid
  (Income Tax Act, winnings from betting and gaming; to be confirmed). **Withdrawals of the wallet are not taxed**
  (`withdrawal_wht` 0%: the money in the wallet is the player's deposits, already charged excise). Both are versioned
  tax rules, so the owner changes them by adding a version, never by editing code.
- **Withdrawal bounds:** KES 50 to KES 150,000 per withdrawal (M-Pesa's B2C ceiling is 250,000); at most 3
  withdrawals per player per Nairobi day.
- **Manual approval:** any payout above KES 100,000 waits (`held`) for an operator, who approves or rejects it with
  `lotteryctl` (back office later).
- **Prizes are paid straight to M-Pesa** (B2C, `PromotionPayment`), as the phase 1 schema (`payouts.kind = prize`,
  one payout per draw result) and the `PrizePayout` posting were designed. A failed prize payout returns the prize to
  its pool and an operator retries it; the prize stays owed. A prize is paid even to a restricted or self-excluded
  player (it is owed); a player in debt is still paid (open question below).
- **B2C fees** are Safaricom's charge to the business; they are not posted (no fee account yet).

## Plan (written before coding)

Rules across every slice:
- **Money out is the mirror of money in.** A payout is recorded and its ledger posting made in one transaction (the
  wallet is debited when a withdrawal is requested, the pool when a prize payout is created); a job sends it.
- **The B2C request is never retried** (`MaxAttempts: 1`): a request whose outcome is unknown may have paid.
- **A result callback is a claim.** Success and failure are both confirmed with Daraja's Transaction Status API before
  the payout is final: a forged success would leave the player unpaid, a forged failure would refund money that left.
  Money comes back to the wallet (or pool) only on a **confirmed** failure: Daraja refused the request outright, or it
  reported a failure that Transaction Status does not contradict. Anything that cannot be confirmed goes to `manual`
  for an operator with the M-Pesa statement.
- Payouts go only to the account's **proved** phone (ADR 0005), and only with verified KYC. Withdrawals are refused to
  a player in debt (ADR 0007). Self-excluded and timed-out players **can** withdraw: their money stays theirs.
- Fail closed: no B2C configuration → withdrawals 503, prize payouts wait.

### Slice 1 — B2C client and mock
`daraja.B2C` (v3 payment request with our OriginatorConversationID), `ParseB2CResult`, Transaction Status by receipt or
by originator conversation id with its own result URLs, B2C configuration (`DARAJA_B2C_SHORTCODE`, sharing the
initiator and credential with C2B). Mock: the B2C endpoint, result and timeout callbacks, faults (refused, outage,
failure result, dropped or duplicated result, a result that lies), Transaction Status for B2C transactions.

### Slice 2 — Withdrawals
Migration (statuses `held → pending → sent | timed_out → result_received → succeeded | failed | manual`, verification
columns, placeholder tax rules), `internal/payouts`: request (eligibility, bounds, daily count, balance, posting,
job), send, result and status hooks (`/hooks/b2c/{secret}/…`), verification loop, settlement (`WithdrawalFailed` on a
confirmed failure), operator actions (`lotteryctl payouts list | approve | reject | settle`), API (`POST
/v1/payouts/withdrawals`, `GET /v1/payouts`, `GET /v1/payouts/{id}`), fault matrix tests through the mock.

### Slice 3 — Prize payouts, tax export, reconciliation, checkpoint
Prize payouts for approved draws (`PrizePayout`, a failure posting that returns the prize to the pool, retry), draws
`approved → paid`, the KRA outbox (withheld tax per paid payout; `lotteryctl tax export`), reconciliation of payouts
(`B2C_FLOAT`, `TAX_PAYABLE`, pools net of prizes paid), checkpoint report.

## Results

### Slice 1 (B2C client and mock)
- `daraja.B2C` (v3 payment request: our OriginatorConversationID, `BusinessPayment` for withdrawals or
  `PromotionPayment` for prizes; refused → `ErrRejected`/`ErrInvalidRequest`, outage or unreadable → `ErrUnavailable`,
  never retried), `B2CStatus` (Transaction Status by receipt or by originator conversation id, answered at the B2C
  status URLs, the payout id as occasion), `ParseB2CResult` (a success must carry a receipt and a whole amount),
  `StatusResult.CreditedMSISDN`. Credentials stay out of errors.
- Config: `DARAJA_B2C_SHORTCODE` turns payouts on; it shares `DARAJA_INITIATOR_NAME` and `DARAJA_SECURITY_CREDENTIAL`
  with the paybill (the initiator without either shortcode, or a shortcode without the initiator, refuses to start).
  Hooks under `/hooks/b2c/{secret}/result|timeout|status-result|status-timeout`.
- Mock: B2C endpoint (initiator, shortcode, command and duplicate-originator checks) with faults: refused, paid but the
  answer lost, failure result, a result that lies, dropped, duplicated, queue timeout, status outage or silence, a
  status amount that differs; Transaction Status answers for payouts, also when the paybill and payout share the
  sandbox shortcode.
