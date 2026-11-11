# 0027. An operating mode: real-money operation needs a licence, two people and a way to stop

Status: accepted
Date: 2026-11-07

## Context
Money moved whenever Daraja credentials were configured: top-ups and payouts started with the credentials
(`internal/platform/config/payments.go`), and plays had no switch at all. Nothing stopped production credentials from
taking real stakes before 4draws holds a licence; running an unlicensed gambling platform is an offence (LN 112/2026
reg 102). Nor could an operator stop money moving in an emergency without redeploying with credentials removed. The
owner decided on 2026-11-07 that the service runs in one of three modes, that the deployed demo may keep running on the
Daraja sandbox while licensing is pending, and that real-money operation needs recorded licence evidence and two
approvals (`docs/release/implementation-roadmap.md`).

## Decision
- **Three modes**, ordered `disabled` < `sandbox` < `live`. `OPERATING_MODE` sets the most the process may do; it
  defaults to `disabled`, and an unknown value refuses to start.
  - `sandbox` refuses to start with production Daraja (`api.safaricom.co.ke` or `DARAJA_ALLOW_PRODUCTION=true`): the
    sandbox, a local mock or no Daraja at all.
  - `live` refuses to start without production Daraja and `REGULATORY_LICENCE_REFERENCE`.
  - Production Daraja with `sandbox` refuses to start; with `disabled` it starts and moves nothing.
- **The database can only lower it.** `compliance.mode_changes` (append-only) records each change with its reason and
  who made it. The effective mode is the lower of the configured mode and the latest change (`sandbox` when there is
  none). `live` takes effect only through a change that carries an approved live request whose licence reference is the
  configured `REGULATORY_LICENCE_REFERENCE`: a configuration change alone never makes the service live, and neither does
  a database row alone.
  A live change that names another licence (or none is configured, as after redeploying as a sandbox) counts as
  `disabled`, so a redeployment never inherits a live state: nothing moves until an admin records the return to
  sandbox, or two admins approve a request for the new licence.
- **Live needs two people.** An admin records a request (`compliance.live_requests`: licence reference, regulator
  approval reference, the SHA-256 and location of the evidence, a reason); a different admin approves it
  (`compliance.live_approvals`, a trigger refuses the requester). The approval writes the `live` change in the same
  transaction. Requests and approvals are append-only. Only the back-office API does this, because staff there are
  authenticated; `lotteryctl` takes a typed staff id and is not trusted for a two-person rule.
- **Anyone on staff can stop; lifting a stop needs an admin.** Stopping (`disabled`) takes one person and a reason, in
  the back office or with `lotteryctl compliance stop`, so it is never slow. Resuming to `sandbox` takes an admin and a
  reason; resuming to `live` takes a new request and approval.
- **Where it is enforced.** `compliance.Gate.Assert` reads the effective mode inside each money transaction and refuses
  in `disabled` with `compliance.ErrOperationsStopped`:
  - starting a top-up (`payments.StartTopUp`) and sending its prompt (`payments.Push`: a recorded top-up whose prompt
    has not gone out fails with `operations_stopped`);
  - paybill validation (Safaricom is told to refuse the payment);
  - a play (`game.Play`);
  - a withdrawal request, approving a held payout, and sending any payout (`payouts.Send`: the payout stays pending and
    the sweep offers it again once operations resume).
- **What it deliberately does not stop.** Money that has already moved is still accounted for: STK callbacks and their
  verification, paybill confirmations, reversals, reconciliation and the operator's recovery actions keep running, so a
  stop never strands a player's money. Draws keep running for entries already paid for (they need two officials
  anyway), and prizes are recorded; their payouts wait at `Send` like any other.
- **Visibility.** `GET /v1/status` (public) gives the mode and whether money is real, so the apps can say "sandbox" and
  "paused". A stop raises the `operations.stopped` page until it is lifted; a live request waiting for its second
  admin raises `operations.live_request`. Every change is in the audit log.

## Consequences
- The deployed service must set `OPERATING_MODE=sandbox` (render.yaml, docker-compose) or it stops taking top-ups and
  plays. Tests that build services without a gate are unaffected; the app wiring always installs one.
- Going live is now a process (configuration plus two recorded approvals plus licence evidence), not a credential.
  The evidence is referenced by hash; the document itself is kept by the operator.
- This is a technical control. It is not a licence, and recording a licence reference does not make one valid.
