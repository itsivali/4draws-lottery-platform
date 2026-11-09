# 0028. M-Pesa callbacks are claims: the first is kept, it must match the prompt, and a late one is confirmed first

Status: accepted (amends 0006 and 0024)
Date: 2026-11-07

## Context
ADR 0006 says an STK callback is a claim and the status query is the proof. The audit of 2026-11-07
(`docs/security/security-audit.md`) found four places where that did not hold:

- **SEC-01.** A paid callback for a top-up that had settled uncredited was credited straight from the callback
  (ADR 0024 §1), and so was an orphan callback an operator rechecked. Anyone holding the hook secret could credit an
  expired top-up with a made-up receipt.
- **SEC-02.** On the normal path the status query proves that the prompt was paid but says nothing about the amount
  or receipt; those came from the stored callback, and every callback replaced the stored one. A second callback could
  supply any amount, and nothing compared it with the amount the prompt asked for.
- **SEC-03.** The hook secret is a URL path segment, and the path went into span names, the `url.path` span attribute
  and the client-IP diagnostic log. Traces leave the platform.
- **SEC-04.** `payments.payments` had no transition rule in the database, unlike payouts (`00023`) and draws.

## Decision
- **The first callback is kept.** `RecordCallback` writes `raw_callback` only when it is empty, and the database
  refuses to change it once set. A repeat of the same callback is ignored. A different one is ignored and audited
  (`payment.topup.callback_ignored`).
- **A callback must describe the prompt.** The customer cannot change the amount of an STK prompt, so a paid callback
  whose amount is not `amount_requested_kes`, or whose receipt is not an M-Pesa receipt (ten letters and digits), did
  not come from that payment. When the status query says paid, such a top-up settles `unconfirmed` with reason
  `amount_mismatch` or `receipt_invalid`, and an operator reconciles it from the statement (ADR 0024). This replaces
  the earlier behaviour of crediting whatever amount the callback said.
- **A late callback is confirmed by M-Pesa first.** A paid callback for a settled, uncredited top-up, or an orphan
  found by an operator's *recheck*, must name the requested amount and an unused receipt. Its receipt then becomes the
  top-up's receipt claim, and the claim job asks Transaction Status. The top-up is credited only when M-Pesa confirms
  a completed payment of that amount, into our business, from the player's number (`settleClaim`, as for a receipt
  the player types).
  - Without the Transaction Status initiator, the claim waits for an operator.
  - A receipt M-Pesa has already answered about is not asked about again.
  - Recheck reports `verifying`.
- **Sandbox exception.** With `DARAJA_SANDBOX_SETTLE` (which configuration refuses with production Daraja), callbacks
  already settle top-ups on the normal path, so a late callback that passes the same amount and receipt checks still
  credits at once. That setting changes no live behaviour.
- **The database enforces the state machine** (`00046`, `payments.check_transition`). Each change below is refused
  with `restrict_violation`:
  - **Fixed at creation:** the player, channel, amount requested, number, idempotency key and creation time never
    change.
  - **Fixed once set:** the prompt's ids, the first callback, and the confirmed amount and receipt.
  - **Status moves:**
    - `reversed` is final.
    - `credited` moves only to `reversed`, and only a credited top-up can be reversed.
    - Pending states only move forward.
    - The uncredited final states (`failed`, `cancelled`, `expired`, `unconfirmed`) may be corrected among themselves,
      or credited when M-Pesa shows a payment.
  - **No deletes:** a top-up row cannot be deleted.
- **The hook secret never reaches telemetry.**
  - `httpx.RedactPath` replaces the secret segment of `/hooks/...` paths with `{secret}`. It also replaces any segment
    that is not a known hook word.
  - The span name and the client-IP diagnostic use the redacted path.
  - A span processor rewrites `url.path` as each span starts, before any exporter sees it.
- **Not done here: an IP allowlist for Safaricom's callback addresses.** It needs a client address the platform can
  trust behind its proxy, which depends on the hosting move (S12, `docs/runbooks/trusted-proxy.md`). It is recorded in
  the roadmap as part of that slice.

## Consequences
- A forged callback can no longer credit anything. What someone holding the secret can still do:
  - A callback that arrives before the real one, for a prompt that was really paid, can put the wrong receipt label on
    the credit, but not the wrong amount. The receipt-uniqueness check and the statement import catch a reused or
    unknown label.
  - A forged failure callback can make a really paid top-up wait for an operator.
  
  Both need the secret, which should be rotated if it leaks (`docs/runbooks/payments-operations.md`).
- Late callbacks credit a few seconds to minutes later, once Transaction Status answers. Without the initiator
  configured, they wait for an operator.
- Tests and fixtures must use well-formed receipts and the requested amount.
