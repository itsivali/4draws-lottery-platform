# 0032. Two people release and settle payouts; operators prove who they are; staff record a player's self-exclusion

Status: accepted (amends 0019, 0023)
Date: 2026-11-09

## Context
The security audit (SEC-05, SEC-06) and the roadmap (OPS-01) found operator powers one person could use alone:

- **Held payouts.** One staff member could release one (above KES 100,000, or held because the account could not be
  paid, ADR 0030).
- **Manual payouts.** One staff member could settle one: "paid" with a receipt nobody checked, or "not paid". The
  second gives the money back, which is a double payout if M-Pesa did in fact pay.
- **`lotteryctl`.** It took the operator as a typed staff id. Anyone holding `DATABASE_URL` could name any staff member,
  so one person could meet a two-person rule (draw sign-off, KRA remittance) by acting once as themself and once as a
  colleague.

Separately, a player who phones to ask to be excluded could only be told to use the app.

## Decision
- **Two approvals release a held payout.** `payouts.Approve` records each staff member's approval in `payouts.approvals`
  (append-only, migration 00051).
  - The payout is sent once two different staff members have approved it. The admin who asked for a refund to source
    (ADR 0031) counts as the first.
  - Approving twice is `ErrSameApprover` (409 `two-person-rule`). Approvers must be signed-in staff (`staff:<id>`).
  - Rejecting stays a one-person action: it sends nothing and gives the money back.
- **Two people settle a manual payout.** The first `Settle` records a proposal in `payouts.settlement_proposals`
  (append-only): paid with a receipt, or not paid with a reason.
  - A second staff member making exactly the same decision (the same receipt for paid) applies it.
  - A different decision replaces the proposal and must be confirmed in turn. The proposer cannot confirm their own.
  - The payout carries its approvals and open proposal in staff views, and the back office shows them.
- **Operators sign in.** `lotteryctl -operator` now takes the staff member's back-office email address. The command
  asks for their back-office password (hidden at a terminal, or the first line of stdin), signs them in exactly as the
  sign-in page does, and records the action under `staff:<id>`.
  - **Refused:** a temporary password, a member without the staff role, and a wrong password.
  - **Google or Apple only:** members who sign in that way set a password before using the CLI.
  - **Logging:** the session is ended when the command finishes; the sign-in is audited.
- **Staff record a self-exclusion the player asks for.** `POST /v1/staff/players/{user}/self-exclusion` with the term
  and how the player asked (`rg.SelfExcludeFor`). It is the player's exclusion: the same terms, sessions ended, and
  audited under the staff member with the reason. There is still no way to lift or shorten an exclusion.

## Consequences
- **Slower releases.** Two people now stand between a held or unconfirmed payout and money moving, so a payout waits
  until both are available. One person's mistake or dishonesty is not enough.
- **Settled receipts are not checked with Transaction Status.** A manual payout reached that state because Transaction
  Status could not confirm it. Asking M-Pesa about the receipt an operator supplies is a possible later addition. Until
  then the second person checking the statement is the control.
- **Residual risk.** Someone with direct database access still bypasses every application control. That includes the
  emergency `lotteryctl staff password -admin`, which can create a member. Database access must stay with as few
  people as possible (S12 moves the database; SEC-08 restores least-privilege roles), and every such change is visible
  in the activity log.
- **Scripts that used `-operator <staff id>`** must change to the email address and supply the password on stdin.
