# 0042. Winners of draw prizes are told by email

Status: accepted
Date: 2026-11-13

## Context

A player learned of a draw prize only by opening the app. The owner asked for win notifications (2026-11-13).
Instant plays need none: the wheel shows the result on the spot.

## Decision

**When a draw is approved, each winner with a confirmed email address is sent one email about each prize.**

- **When:** a worker job (`winnotice.notify`, every 5 minutes) looks for prizes of approved draws that have no
  record in `draws.prize_notices`. A conducted draw is not announced until two officials have approved it.
- **To whom:** the address on the winner's account, only if confirmed (ADR 0040). No confirmed address, or a closed
  account: no email, and the reason is recorded. The winner still sees the prize in the app and is still paid.
- **Once:** one record per prize (`emailed`, or `skipped` with the reason), written in the same transaction the
  email is sent in, and audited as `draws.prize_notice`. Prizes won before this existed are marked skipped by the
  migration, so nobody is written to about the past.
- **If the mail server refuses it:** nothing is recorded and the next run tries again, for 72 hours; then the prize
  is recorded as not announced.
- **What it says:** the amount, the draw and its period, how the prize is paid (to the account's M-Pesa number,
  after winnings tax, large prizes after a staff check), that payment needs a verified identity, and a link to the
  draw and its proof. It states facts and does not invite more play.
- **Without a mail server** the job is not registered.

## Consequences

- A player who has taken a break or excluded themselves is still told: the prize is money owed to them.
- There is no setting to switch the notices off. Add one before any message that is not about money owed.
- SMS is not sent: no gateway is configured (`internal/notifications` has the adapter).
- The email names the amount before tax. The exact tax is on the payout, in the app.
