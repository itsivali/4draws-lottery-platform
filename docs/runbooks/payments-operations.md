# Runbook: payments operator actions

Operators use the back office (`docs/runbooks/back-office.md`, *Payments*), audited as `staff:<id>`. The `lotteryctl`
commands below remain for emergencies, with the database URL (the external URL from the Render dashboard; never commit
it); they take `-operator <your staff id>` and are audited as `staff:<id>`.

```sh
export DATABASE_URL='postgres://...'          # external connection string of lottery-db
go run ./cmd/lotteryctl payments reconcile    # prints the report; exit 1 if there are mismatches
```

## The daily reconciliation

Runs in the api's job workers every 24 hours (and on start); each run is stored in `payments.reconciliation_runs` and
logged: `ERROR payments reconciliation found mismatches` or `WARN ... items need an operator`.

- **Mismatches** break an invariant between payments, ledger and wallets (`credited_without_deposit`,
  `deposit_without_credit`, `deposit_amount_mismatch`, `reversal_mismatch`, `clearing_vs_credited` (now credited +
  open suspense), `liability_vs_wallets`, `suspense_vs_items`, `suspense_without_journal`). They should never happen: stop and investigate (a bug or tampering).
- **Attention** lists work for an operator:
  - `unconfirmed`: settlement gave up without M-Pesa's confirmation (`paid_without_callback`: the status query said
    paid but no receipt arrived; `still_pending_at_expiry`; `query_rejected`). Open the top-up in the back office
    (Top-ups → the row): a late callback or the player's receipt may already have settled it. Otherwise import the
    M-Pesa statement (below), or resolve it from the receipt by hand, or dismiss it as not on the statement.
  - `receipt_claims`: receipts players gave for uncredited top-ups that M-Pesa has not confirmed (`claimed`,
    `checking`) or did not confirm (`unverified`, with M-Pesa's reason on the top-up). Check the statement for the
    receipt and resolve or dismiss.
  - `overdue_pending`: pending past the expiry window: the sweep should settle these; persistent ones are a bug.
  - `orphan_callbacks`: callbacks for pushes we never recorded. Check the statement for a matching receipt.
  - `players_in_debt`: players who owe after a reversal; recovered automatically from their next top-ups.
  - `suspense_open`: paybill payments Daraja confirmed that could not be credited (see below).
  - `c2b_unverified`: paybill confirmations Daraja did not confirm. Check the statement; if the payment is there,
    fix the cause (initiator, credential) and run `c2b-recheck`.
  - `c2b_overdue`: paybill confirmations still being verified after an hour: Transaction Status is not answering.
  - `c2b_conflict_after_credit`: a second, different confirmation arrived for a receipt already credited. Possible
    forgery: compare with the statement, reverse if credited to the wrong player, and rotate `DARAJA_CALLBACK_TOKEN`.

### A top-up that stays "Waiting for M-Pesa"

After the customer's PIN, Daraja confirms in one of two ways: its callback to `/hooks/mpesa/stk/…` (logged as
`stk callback received` with the checkout id and result code since 2026-10-30; an orphan, one for a payment we do
not know, is logged as a warning) or, failing that, the api's status query every `VerifyAfter` (2 minutes) until the
payment expires (15 minutes), after which it is settled `unconfirmed`. The player app shows `pending` as "Waiting for
M-Pesa" and `unconfirmed` as "Not confirmed yet" with what to expect (credited from the statement if M-Pesa took the
money; nothing paid otherwise), so nobody is left waiting on a prompt that will not answer. A payment has one verify job at a time
(before 2026-10-30 the push, the callback and the sweep each ran one, tripling the queries); a callback brings it
forward. Daraja throttles status queries (HTTP 429, `daraja: unavailable: throttled`); a throttled query waits twice
as long before asking again. A throttled push (`stk push throttled; asking again`) is refused before any prompt is
sent, so it is asked again after 2, 4 and 8 seconds; after that the top-up fails as `push_throttled`. Production
Daraja answers within seconds; the **sandbox** sends real prompts but often calls back a timeout (1037) for an
answered prompt, or nothing, and throttles queries routinely.

### Sandbox settlement (`DARAJA_SANDBOX_SETTLE=true`)

The Daraja sandbox carries real M-Pesa payments: an approved prompt deducts the money, M-Pesa sends its usual
confirmation SMS with a real receipt, and Safaricom reverses the payment a few hours later (owner, 2026-10-30;
Daisy Irungu's KES 1 on 2026-10-29, receipt UJT6YBR9IQ). It throttles the status query (429), so with
`DARAJA_SANDBOX_SETTLE=true` a top-up is settled from Safaricom's callback without asking, and only real payments are
credited:

| Callback | Settled as |
| --- | --- |
| paid (0) | credited with the callback's amount and Safaricom's receipt; proves the number, as in production |
| cancelled by the customer (1032) | cancelled |
| any other code (1037 the phone did not answer, 2001 wrong PIN, 1 insufficient balance...) | failed, nothing credited |
| none, or malformed | the status query decides, as without the setting (credited only with a callback's receipt) |

Safaricom's later reversal of a sandbox payment is not reported to the platform: the wallet keeps the credit. Before
2026-10-30 afternoon, sandbox settlement also credited timed-out prompts under a `SANDBOX-<payment id>` receipt; those
rows stay, marked *Sandbox* on the back office's Top-ups page, and migration 00040 proved those accounts' numbers.

It is set in render.yaml and docker-compose.sandbox.yml; the api logs `sandbox_settle=true` at start. **The api refuses
to start with it and production Daraja** (`DARAJA_ALLOW_PRODUCTION` or `api.safaricom.co.ke`), where the status query
confirms every payment: delete the key from render.yaml and the dashboard when switching to production.

If every prompt to one number times out (1037) about 26 seconds after the PIN, with no deduction and no SMS, the PIN
is not reaching M-Pesa: check that the line is the phone's M-Pesa SIM, try another number, or ask Safaricom (100).

## A top-up M-Pesa completed but the platform never credited (ADR 0024)

Money is credited only against an M-Pesa receipt. For a top-up that settled uncredited the receipt comes from one of:

1. **A late callback.** Safaricom retries callbacks; one that arrives after the top-up settled credits it at once
   (audit `payment.topup.credited`, `source: late_callback`). Nothing to do.
2. **The player's M-Pesa message.** The app asks the player for the receipt code once a top-up settles uncredited
   (`POST /v1/payments/{id}/receipt`). With the paybill initiator configured (`DARAJA_C2B_SHORTCODE`,
   `DARAJA_INITIATOR_NAME`, `DARAJA_SECURITY_CREDENTIAL`) the platform asks M-Pesa about the receipt (Transaction
   Status) and credits the top-up when M-Pesa confirms a completed payment of the amount asked, to our business, from
   the player's number. A claim M-Pesa does not confirm is `unverified` with the reason; the sweep asks again every
   10 minutes while M-Pesa has not answered. Without the initiator the claim waits for you.
3. **The M-Pesa statement.** Back office → Payments → *Import an M-Pesa statement*: upload Safaricom's CSV export
   (organisation portal → Completed transactions → Export). Every paid row is matched: `credited` (known receipt),
   `claimed` (the receipt a player gave), `matched` (one prompt with the same amount, number and time: 2 h before
   to 20 min after the prompt), `ambiguous` (pick one), `unmatched`. Credited prompts in the window that the
   statement does not show are listed as *missing from statement*: either the file is partial or a credit is wrong.
   Nothing is credited by the upload; confirm the list and each credit stands alone. The import is kept
   (`payments.statement_imports`: file hash, counts, report, outcomes) and audited (`payment.statement.imported`).
4. **You, one top-up at a time**, from the top-up's page in the back office (`GET /v1/staff/topups/{id}` shows the
   callback, the claim and M-Pesa's answer, orphan callbacks, the jobs, the audit trail, the player's standing):
   - *Ask M-Pesa again* (`recheck`): credits an orphan or late callback; otherwise reports the status query:
     `paid_without_receipt` means the receipt is still needed; a failure code is recorded.
   - *Credit from a receipt* (`resolve`): with the receipt and the amount on the statement, and a note.
   - *Not on the statement* (`dismiss`): the top-up fails as `not_on_statement`; a claimed receipt is rejected with
     your note. It can still be credited later if it does turn up.

From the command line:

```sh
go run ./cmd/lotteryctl payments resolve -payment <payment id> -receipt <M-Pesa receipt> -amount <KES on the statement> -operator <your staff id>
```

Credits exactly like a confirmed callback (excise booked to the operator, debt recovered, phone proved). Refused if the
payment holds money already or is still being settled, if the receipt is not 10 letters and digits, or if the receipt
already credited another payment. A receipt credits one payment, ever.

## Reverse a top-up M-Pesa took back

```sh
go run ./cmd/lotteryctl payments reverse -receipt <M-Pesa receipt> -reason "<why>" -operator <your staff id>
```

Takes back what the player received (ADR 0007): from the wallet as far as it goes, the rest as debt. Refused unless
the payment is `credited`; a payment can be reversed once.

## Paybill (C2B) payments

Paybill payments are verified with Daraja's Transaction Status API before anything is credited (ADR 0008). Each one
ends `credited`, in **suspense**, or `unverified`. The reason is in `payments.c2b_confirmations.outcome_reason` and,
for suspense, `payments.c2b_unmatched.reason`: `unknown_account` (the account number is no proved phone),
`kyc_required`, `account_suspended`, `restricted` (time-out or self-exclusion), `over_deposit_limit`, `bad_amount`,
`amount_mismatch`, `payer_mismatch`, `conflicting_confirmations` (two confirmations disagreed: possible forgery).

### Register the paybill URLs (once per shortcode)

Needs the api's Daraja variables, the three C2B variables, `DARAJA_CALLBACK_TOKEN` and `PUBLIC_BASE_URL` (copy them
from the Render dashboard into your shell; never commit them):

```sh
go run ./cmd/lotteryctl payments c2b-register
```

The sandbox accepts repeated registrations. In production a shortcode registers once; changing URLs needs Safaricom.
A successful registration answers `ResponseCode "00000000"` (not `"0"`); the sandbox paybill is 600992, the Party A
on the app's Test Credentials page. As of 2026-10-24 the sandbox accepted registrations, simulations and status
requests but did not call the hooks back; if confirmations never arrive, check the hooks from outside first (a `{}`
POST to `/hooks/c2b/<secret>/validate` must answer `C2B00016`) before suspecting the app.

### Credit a suspense item to the right player

After identifying the payer (for example they call support and prove the account is theirs):

```sh
go run ./cmd/lotteryctl payments suspense-credit -receipt <M-Pesa receipt> -user <user id> -operator <your staff id>
```

Refused for players who may not receive it (KYC, suspended, time-out or self-exclusion, deposit limit): refund those.
It credits like any deposit (excise booked to the operator, debt recovered) and never proves a phone.

### Refund a suspense item

Send the money back from the M-Pesa org portal (or a B2C payment), then record it with that transaction's reference:

```sh
go run ./cmd/lotteryctl payments suspense-refund -receipt <M-Pesa receipt> -refund-ref <refund reference> -operator <your staff id>
```

Self-excluded and timed-out players' payments must be refunded, not credited.

### Check an unverified paybill payment again

```sh
go run ./cmd/lotteryctl payments c2b-recheck -receipt <M-Pesa receipt> -operator <your staff id>
```

The worker asks Daraja again within a minute or two.

### Sandbox end-to-end

```sh
go run ./cmd/lotteryctl payments c2b-simulate -amount 10 -msisdn 254708374149 -account <test player's proved phone>
```

The sandbox only pays from its test number; the account number decides who is credited.
