# 0030. Eligibility is checked again when money moves; status changes are kept; self-exclusion signs out

Status: accepted (amends 0005, 0022)
Date: 2026-11-08

## Context
Eligibility was decided once, when a request was recorded, and not again when money actually moved:

- **Payouts.** `RequestWithdrawal` and `PayPrizes` check `identity.AssertCanBePaid`, but the B2C request comes later,
  from the send job, or from an operator approving a held payout. An account suspended, closed, or whose ID lost its
  verified status in between was paid anyway, to the number recorded on the payout.
- **Top-up prompts.** `StartTopUp` checks the account and responsible-gaming restrictions. The push job, which sends
  the M-Pesa prompt seconds or minutes later, checked only the operating mode (ADR 0027). A player who self-excluded
  in that window still got a prompt and could pay.
- **Status changes** (active, suspended, closed) left only audit events, written by the code paths that remembered to,
  and nothing for a change made any other way.
- **Self-exclusion** left the player signed in. The app stopped play, but the session that excluded itself stayed
  valid on every device.

The compliance gap analysis lists these as ID-02 and ID-03 (`docs/release/implementation-roadmap.md`).

## Decision
- **Payouts are checked as money is about to leave.** The send job (`payouts.Send`) and `payouts.Approve` call
  `AssertCanBePaid` again, inside the transaction that claims the payout. They also require the account's proved number
  to still be the one the payout names. Money only goes to the proved number (ADR 0005), and the payout's number cannot
  change.
  - **When the send job finds the account cannot be paid,** nothing is sent. The payout goes back to `held`, with
    `payee_ineligible: …` as the reason, and a `payout.held` audit event is written. The money stays reserved.
    Migration 00048 allows `pending → held`, the only new transition.
  - **When an approval finds it,** the approval is refused (409 `payee-ineligible`, the detail says why) and the
    payout stays held.
  - **Either way, an operator decides.** They can approve once the account may be paid again, or reject, which
    returns the money to the wallet (or the prize to its pool, still owed).
- **The prompt job decides again what `StartTopUp` decided.** Just before the prompt, `payments.Push` re-checks four
  things:
  - the operating mode (`operations_stopped`);
  - `AssertCanTopUp` (`account_ineligible`: suspended, closed, or with real money an unverified ID, ADR 0029);
  - `rg.AssertNotRestricted` (`player_restricted`);
  - the claimed number still being the one recorded (`number_changed`).

  A top-up that fails any of them fails without a prompt. Deposit limits are not re-checked, because `StartTopUp`
  already counted this top-up against them.
- **Every status change is kept.** `identity.user_status_history` (migration 00049) is append-only. An `AFTER INSERT
  OR UPDATE OF status` trigger on `identity.users` writes it, so no code path can change a status without leaving a
  row.
  - **Who and why.** The services name the actor and reason for their transaction (`NameStatusChange`, transaction
    settings `identity.status_actor_type`, `identity.status_actor`, `identity.status_reason`). Suspend, reinstate and
    close do this.
  - **Other rows.** A sign-up is recorded as the player's own. A change made outside the services is still recorded,
    with the actor and reason left empty. Existing accounts get one row: their status when the history began.
  - **Where it shows.** The back office player timeline and the player pack.
- **Self-exclusion signs the player out everywhere, at once.** `rg.SelfExclude` ends every session in its own
  transaction, through `identity.EndSessionsForSelfExclusion`, with `revoked_reason = 'self_exclusion'`.
  - **Signing in again is allowed,** so the player can withdraw what is theirs. Plays and top-ups stay refused.
  - **The app** forgets the session at once and says on the sign-in screen that the player self-excluded, rather than
    that the session timed out.
  - **A time-out is unchanged.** It is a pause, and keeps the session.

## Consequences
- **Delays.** A payout to an account that cannot be paid now waits in `held` for a person. It is never sent there, and
  never failed automatically. Operators see it in the held queue with its reason (`docs/runbooks/payouts.md`).
- **Prizes.** A held prize whose winner stays ineligible is rejected by an operator. The prize goes back to its pool,
  still owed (`RetryPrize` pays it later).
- **Clients.** The self-exclusion request's own tokens stop working once it returns. Clients must expect a 401 after
  it; the app built from this change handles that.
- **Still to do in S3:** a way out for balances that cannot be paid (refund to source), and a location signal (S3c).
