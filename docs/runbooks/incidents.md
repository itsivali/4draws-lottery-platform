# Runbook: incidents

What to do when something goes wrong with real money or real draws. Act to stop harm first, then keep evidence, then
fix. Every staff action is audited; write what you did in the alert's acknowledgement note or the incident record.

## First five minutes (any incident)

1. **Say it out loud:** post in the on-call channel what you see, when it started, and who is handling it.
2. **Stop money moving if in doubt.** A failed reconciliation already freezes payouts (`docs/runbooks/alerts.md`). To
   stop new plays and top-ups as well, scale the api to zero on Render (players then see the app's offline state;
   nothing is lost: every write is idempotent and retried with the same key).
3. **Keep evidence:** do not delete rows, restart databases or rotate keys before noting the time, the alert, and the
   request ids you have.

## Payments (M-Pesa)

| Symptom | Likely cause | Do |
|---|---|---|
| Top-ups stay *pending* | Daraja down or slow; callbacks not reaching us | Check Daraja status; check `/hooks/...` reach the api (Render logs). Nothing to retry by hand: verification asks Daraja again. Unconfirmed top-ups are resolved from the statement (back office → Payments → *Resolve*). |
| A player says they paid but nothing arrived | Unconfirmed payment, or a paybill payment in suspense | Run the payments reconciliation; resolve from the statement receipt; credit suspense to the right player. Never credit by hand outside these actions. |
| M-Pesa reverses a payment | Dispute or fraud | Back office → Payments → *Record a reversal*. The player's debt pauses play and withdrawals. |
| Payouts stuck *processing* or in *manual* | B2C answer unclear | Settle from the M-Pesa statement (back office → Payouts). Never send a second payout for the same withdrawal. |

## Draws

| Symptom | Do |
|---|---|
| A draw is late (alert `draw overdue`) | Officials sign before the drand round (09:00 Nairobi). If the round passed unsigned, the draw is voided and re-run automatically. |
| drand unreachable at the round | The draw waits; it is conducted when the round can be fetched and verified. Do not void unless counsel advises. |
| Someone suspects a result | Publish nothing new; point them to the certificate and `tools/verifier draw`; the regulator holds the entry list. |

## Security

| Symptom | Do |
|---|---|
| A staff account may be compromised | Disable the user in Keycloak (its tokens expire within 10 minutes); review the audit log for `staff:<id>`; check two-person steps it took. |
| A player account may be taken over | Suspend the account (support, back office later; today `UPDATE identity.users SET status = 'suspended'` by the owner with the incident noted); sessions end with it. |
| A secret may have leaked (API keys, Daraja, database) | Follow `docs/runbooks/secrets-and-keys.md` to rotate; Daraja credentials first, then the Render API key. |
| Unusual sign-up or play volume | Check rate-limit 429s and per-address patterns; consider `ATTESTATION_MODE=refuse_failed` and `ATTESTATION_WEB=refuse` once attestation is live (ADR 0014). |

## Data

| Symptom | Do |
|---|---|
| The database is unreachable | Render status; the free database rotation (`docs/runbooks/free-database-rotation.md`) if it expired. The api answers 503 and the apps show offline; no money moves. |
| A reconciliation reports a mismatch | Payouts are frozen. Do not acknowledge until the cause is found; the mismatch names the rows. |

## After

Write a short record: timeline, impact (players, money, draws), cause, what changed. Players affected by a money
error are told what happened and what was corrected. Report to the regulator when the licence requires it (owner).
