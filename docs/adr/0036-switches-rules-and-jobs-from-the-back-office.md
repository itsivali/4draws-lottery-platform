# 0036. Money switches, monitoring rules and background jobs from the back office

Status: accepted
Date: 2026-11-11

## Context

The owner asked for a System page with working toggles, monitoring rules that can be changed and resolved by clicking
them, and background jobs that can be understood and acted on (2026-11-11). Until now:

- Everything in "Settings" came from the deployment's configuration, and the only runtime control was the operating
  mode (ADR 0027): stop everything, or resume.
- The monitoring rules (ADR 0033) changed only through `lotteryctl aml set-rule`.
- A failing job could be seen but not retried or cancelled.

## Decision

1. **Money switches.** `compliance.area_switches` (append-only) pauses one area: top-ups (new prompts and paybill
   payments), plays, withdrawals, or payouts (B2C sends).
   - The gate's `Assert` refuses an operation of a paused area with the same `ErrOperationsStopped` as a stop, so
     every caller already handles it: players get `operations-stopped`, and a payout waits pending for the sweep.
   - A switch only restricts. It can never allow what the configuration or the operating mode refuses.
   - Anyone on staff may pause; only an admin resumes. This is the rule ADR 0027 set for stop and resume.
   - Each change needs a reason and is audited (`compliance.area_paused` / `compliance.area_resumed`).
   - A paused area raises a warning alert (`operations.area_paused`) until it is resumed.
   - `GET /v1/status` lists the paused areas so the apps can say so.
   - Deployment settings (M-Pesa credentials, sign-in providers, email) stay configuration. They hold secrets and
     belong to the hosting dashboard, so the page shows them read-only.
2. **Monitoring rules.** An admin records a rule's setting from the back office (`PUT /v1/staff/aml/rules/{rule}`): on
   or off, its thresholds, a note, and the compliance owner's approval reference.
   - This is the same `SetRule` that `lotteryctl` uses, with the same checks and audit.
   - "Record approval" resolves a pending rule.
   - Editing thresholds asks for the reference again, so an approval is never carried over to values the owner did not
     see.
3. **Background jobs.**
   - The System page names each kind of job and says what it does, how often it runs, when it last finished and when it
     next runs.
   - An admin can retry a job that is retrying, discarded or cancelled, or cancel one that is waiting or retrying.
     Both are audited (`system.job_retry` / `system.job_cancel`).
   - A completed job is never run again.
   - A retry is safe because every job checks its record's state before acting: a payout is sent only while it is
     pending, and it is marked sending before the request is made.

## Consequences

- An outage or a fault in one area no longer means stopping everything.
- Two people are still needed to put an area back: one pauses, an admin resumes. This is the same as stop and resume.
  It is not the two-different-admins rule of going live.
- Rule changes from the back office are recorded like those from `lotteryctl`. The compliance owner's approval remains
  a reference to their own record, not a sign-off inside the system.
