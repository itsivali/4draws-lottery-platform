# 0023. Finance reporting by Nairobi period, a natural-sign trial balance, and audited exports

Status: accepted
Date: 2026-10-31

## Context
The back office's Finance page showed thirty days of ledger figures and nothing an auditor could take away. The
owner asked (2026-10-31) for "a rich finance tab" that would "also help with audits": figures by week and month as
well as by day, the top-up and payout funnels, the trial balance, the tax position, the stored reconciliation runs,
and files. Every money figure on the page already came from ledger postings (`internal/finance`), which is what makes
it worth exporting: it agrees with the reconciliations by construction.

## Decision
- **Periods are Nairobi periods.** A range is two Nairobi dates, inclusive, at most 366 days apart. `group=day`,
  `week` or `month` cuts it into Nairobi days, Monday-start weeks (`date_trunc('week')`) or calendar months; a first or
  last period the range cuts into is clipped to the range and says so by its `start` and `end`. Postgres buckets the
  rows that need distinct counts (players per period); Go buckets the rest the same way. Money is summed from ledger
  entries by journal kind and account, as before; the funnels come from `payments.payments` and `payouts.payouts`
  by the Nairobi day the request was made.
- **The trial balance carries natural signs.** Every ledger account is listed with its debits, credits and balance as
  at the end of the range's last day; the balance is debits less credits for assets and expenses and credits less
  debits for liabilities and revenue, so an operator reads "owed" as a positive number. The sum of all debits must
  equal the sum of all credits (double entry, enforced per journal by a database trigger); `balanced: false` is a
  broken posting and the page says so with a warning, never silently.
- **Two CSV exports, both audited.** `GET /v1/staff/finance/export` is the page: one row per period, a blank line, the
  totals, a blank line, the trial balance and whether it balances. `GET /v1/staff/ledger/export` is every ledger
  entry of the range, oldest first (journal, kind, reference, time in EAT, account, side, amount, player), **capped
  at 100,000 rows**; more is refused with `too-many-rows` and the operator narrows the range, so one request can
  never hold the database or the browser hostage. The ledger file streams from one repeatable-read snapshot and is
  hashed from that snapshot before the first byte is sent, so `X-File-SHA256` is a real header (the tax files'
  convention) and not a trailer browsers cannot read. Every export, whatever its size, writes a staff audit event
  (`finance.exported`, `ledger.exported`) with the range, the row count and the hash before the file leaves.
- **Stored reconciliation runs are listed** (`GET /v1/staff/reconciliations?area=`) with counts of findings per kind,
  from the append-only `reconciliation_runs` tables; the findings themselves stay in the run's report.

## Consequences
- A ledger export reads its rows twice (once to hash, once to send) inside one snapshot: twice the database work, no
  memory beyond a buffered writer, and a hash the audit trail and the auditor can both check.
- The row cap is a product limit, not a performance one; an auditor who needs a year of a busy ledger exports it
  month by month, and the audit trail shows each piece. Raising it is a one-line change once the volume justifies it.
- The response of `GET /v1/staff/finance` grew (`periods` replaced `days`; `by_kind`, `trial_balance`, `top_ups`,
  `payouts`, `prize_pools` and `tax` were added). It reads about ten queries per call; it is a staff page, polled by
  nobody, so that is acceptable, and every query is bounded by the 366-day range or by the account list.
- Figures the back office shows and figures in the files are the same code path (`Summarize`), so they cannot drift.
