# Phase 5 — Payouts: checkpoint

Status: COMPLETE in code (2026-10-25, built on the owner's instruction to go up to the web app); not yet run live in the sandbox.

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
  player (it is owed). **A prize first repays the winner's debt** from a reversed top-up (owner's decision,
  2026-10-25): tax on the whole prize, the debt from what is left, the rest sent; wholly absorbed prizes send nothing
  (`offset` route, migration 00025).
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

### Slice 2 (withdrawals)
- Migration `00023`: payout states `held → pending → sending → sent | timed_out → result_received → succeeded |
  failed | manual`, forward-only (trigger), amounts, payee and keys immutable, final states final, no deletes; a
  receipt for every success; `payouts.b2c_callbacks` (append-only; the api may append but never change a payout);
  placeholder tax rules `winnings_wht` 20% and `withdrawal_wht` 0%; a failed prize payout no longer blocks a retry.
- `internal/payouts`: `RequestWithdrawal` (proved phone and verified KYC via `identity.AssertCanBePaid`, no debt,
  KES 50–150,000, 3 a Nairobi day, balance, posting and job in one transaction under a per-player lock; above KES
  100,000 held), `Send` (claims `sending` first, never repeats the request; refused → failed and refunded, unknown →
  timed_out), `HandleCallback` (stores and enqueues; found by originator, or by occasion/status originator), `Verify`
  (waits for the result, asks Transaction Status by our originator id, never by a receipt a result claims; completed
  with the right amount to the right number → succeeded; a failure Daraja reported and Transaction Status does not
  contradict → failed and refunded with the payout's own tax rule; otherwise asks again, then manual), `Sweep`
  (re-queues a pending payout, turns a stuck `sending` into timed_out), operator `Approve | Reject | Settle`.
- River uniqueness for payout jobs ignores finished jobs (River's default counts completed jobs, which would have
  swallowed a send after an approval or a sweep).
- API: `POST /v1/payouts/withdrawals`, `GET /v1/payouts`, `GET /v1/payouts/{id}` (player sees processing/paid/failed),
  hooks `/hooks/b2c/{secret}/…`; `lotteryctl payouts list | approve | reject | settle-paid | settle-failed`; runbook
  `docs/runbooks/payouts.md`; `DARAJA_B2C_SHORTCODE` in the Blueprint and compose.
- Tests through the mock: paid and confirmed once (replay debits nothing); refused; paid with the answer lost; failure
  result; queue timeout; a lying success (→ manual); result dropped or duplicated; another amount (→ manual); outage
  with nothing sent (→ manual, not refunded); status outage retried; an interrupted send checked and never re-sent; a
  lost send job re-queued; eligibility and limits (bounds, balance, phone, KYC, debt, daily count); refused without
  B2C; held, approved, rejected; manual settled from the statement; six concurrent requests never overdraw; the
  database refuses changed amounts or payee, skipped states, changes to a final payout, deletes, and the api role
  changing a payout; a receipt that already paid another payout, money credited to another number and an answer to an
  older status request are not trusted; a rejected status request goes to an operator; a failing clock in the middle
  of any step leaves no debit, payout or state change behind; the queue alone pays a withdrawal. Found and fixed: a
  status answer with a malformed occasion aborted the hook's transaction (500); a reused receipt hit the unique
  constraint and aborted the settlement instead of handing the payout to an operator (now checked first).
- Coverage: payouts 86.6%, overall 86.3%; `make ci` green.
- Hand mutations killed: refunding on any unconfirmed status; succeeding without the amount check.
- Open: Daraja's sandbox support for Transaction Status by `OriginalConversationID` must be seen live (the mock
  implements it); until then a payout without a confirmed status lands in manual, never refunded.

### Slice 3 (prize payouts, tax feed, reconciliation)
- `ledger.PrizePayoutFailed` (exact reversal of `PrizePayout`: the prize back to its pool, the tax released).
- `payouts.PayPrizes` (job `payouts.prizes`, every 5 minutes): a payout per prize of an approved draw that never had one
  (`winnings_wht` withheld, pool → B2C float and tax, queued or held), skipping winners who cannot be paid now; a
  failed prize goes back to its pool and only an operator pays it again (`RetryPrize`, `lotteryctl payouts
  retry-prize`). The draws schedule marks an approved draw `paid` once every prize has a confirmed payout.
- KRA feed: one `withholding_tax` event per paid payout with tax (unique index, migration `00024`); `ExportTax` /
  `lotteryctl tax export` writes them as JSON lines and marks them exported only when the write succeeded.
- Reconciliation (`payouts.reconciliation_runs`, daily job, `lotteryctl payouts reconcile`): journals per payout and
  their amounts, failure reversals, prize amounts, KRA events, `B2C_FLOAT` and `TAX_PAYABLE` against live payouts; the
  draw reconciliation now subtracts prizes paid out from the pools.
- Tests (coverage payouts 86.3%, draws 85.1%, overall 86.3%; `make ci` green): prizes paid with 20% withheld (105 → 21 + 84; 63 → 13 + 50), pool emptied, float and tax booked, reported to
  the feed once and exported once, the draw marked paid; a failed prize back in its pool, not retried by itself, paid
  again by an operator (a second retry refused while one is live); a suspended winner skipped; a large prize held;
  nothing created without B2C; tampering caught (a missing tax event, a failure without its reversal, a rewritten
  journal, a float that does not add up); pool reconciliation with prizes paid out and the draw paid only when the last
  prize is. Hand mutation killed: a failed prize that is not given back to its pool.

## Checkpoint (2026-10-25)

Phase 5 is complete in code: withdrawals and prizes leave through M-Pesa B2C, each recorded and posted before it is
sent, sent once, and settled only on a Transaction Status confirmation; refunds only on a confirmed failure; operators
handle what cannot be confirmed; tax withheld on prizes is reported to the KRA feed; payouts and pools reconcile daily.
Not yet seen live: a sandbox B2C payout (needs `DARAJA_B2C_SHORTCODE` on Render) and Daraja's sandbox answer to a
Transaction Status request by originator conversation id. Open for the owner and a tax adviser: the withholding rates
(placeholders), the withdrawal bounds and daily count, the manual threshold, and how the KRA feed is to be submitted.
Settled since: a prize first repays the winner's debt (2026-10-25).
