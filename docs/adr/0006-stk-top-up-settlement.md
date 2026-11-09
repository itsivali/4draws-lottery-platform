# 0006. How an STK top-up is settled

Status: accepted; amended by 0028 (the first callback is kept and must name the requested amount)
Date: 2026-10-22

## Context
An STK Push top-up crosses three unreliable boundaries: our push request (which can time out after Safaricom accepted
it), Daraja's callback (which can be lost, duplicated, delayed or forged by anyone who learns the URL) and the status
query (which answers "still processing" for a while, sometimes as HTTP 200 with result code 4999). The same top-up
also proves the player's phone (ADR 0005), so a mistake either loses money, credits money that was never paid, or
proves a phone the player does not hold.

## Decision
- **A callback is a claim; the status query is the proof.** Money moves only when the query says paid. The amount and
  receipt come from the stored callback (the query does not return them), so a paid query with no callback credits
  nothing: after the expiry window the payment becomes `unconfirmed` for reconciliation against the M-Pesa statement.
- **The push is never retried.** A timeout or 5xx may still have reached the customer; a second prompt could take a
  second payment we would not know about. The payment fails (`push_outcome_unknown`) and any callback it later
  produces is kept in `payments.stk_orphan_callbacks`.
- **Exactly-once credit**: payment row lock + final-state check, the unique receipt, the deposit journal keyed on the
  payment id, and a receipt already credited on another payment is refused (`receipt_reused`, unconfirmed).
- **The phone proved is the push target** (the number on the account, never one the client sends). A payer number the
  callback reports that differs from it is treated as suspicious and proves nothing; a masked or missing one is
  ignored. A failure to prove (number already proved by another account) never undoes the credit; it runs in a
  savepoint.
- **Verification jobs are unique per payment** (since 2026-10-30; before, per payment and source — `push`,
  `callback`, `sweep` — which ran up to three status-query chains per payment and fed Daraja's throttling). A callback
  brings the payment's waiting job forward (River `JobRetry`), so it is still never held back behind a snoozed
  safety-net job.
- **A throttled push is asked again** (2026-10-30): HTTP 429 is Daraja's spike arrest, which refuses the request before
  any prompt is sent, so the double-charge risk above does not apply. Three more attempts after 2, 4 and 8 seconds,
  then `push_throttled`.
- **Sandbox settlement** (`DARAJA_SANDBOX_SETTLE`, owner, 2026-10-30): the Daraja sandbox carries real M-Pesa
  payments (Safaricom reverses them later) but throttles the status query, so a top-up settles from Safaricom's
  callback alone: paid credits with its receipt, 1032 is cancelled, any other code fails; without a usable callback
  the status query decides. Nothing is credited without Safaricom's receipt. (For a few hours on 2026-10-30 timed-out
  prompts were also credited under `SANDBOX-` receipts; the owner chose real payments only.) Refused with production
  Daraja, where the query confirms every callback.
- **The callback URL's secret segment is sha256(DARAJA_CALLBACK_TOKEN) in hex**, because a Render-generated base64 secret
  may contain `/`. The route lives outside `/v1` (no token, no Idempotency-Key) and compares in constant time.
- **Limits count top-ups in flight**, under a per-player advisory lock, so parallel requests cannot pass a deposit limit.
- Excise is computed at the rate version in force when the credit is posted. It was deducted from the deposit (owner,
  2026-10-22); since 2026-10-26 the operator pays it and the player is credited in full (ADR 0016).

## Consequences
- A lost callback delays a real payment until an operator supplies the receipt from the statement. Players see `pending`
  while settlement is still trying and `unconfirmed` once it has given up (2026-10-30), so they are neither told to keep
  waiting on M-Pesa nor that nothing was taken.
- A push outage costs the player a retry rather than risking a double charge.
- Without Daraja credentials or an excise version in force, top-ups are refused (503) rather than credited untaxed.
