# Runbook: withheld tax and KRA remittance

Every paid payout with tax withheld (prizes: placeholder 20%, `winnings_wht`) adds one `withholding_tax` event to
`audit.kra_outbox`: payout, payee id, gross, tax, debt repaid, net, rate, receipt, time. Events are never changed or
deleted. They reach KRA through **remittance batches**, prepared by one person and remitted by another.

## The cycle

1. **Prepare** (any operator): freeze every event not yet batched into the next numbered batch. Batch numbers have
   no gaps. The batch records its control totals (events, gross, tax, first and last event) and the SHA-256 of its file.

   ```sh
   go run ./cmd/lotteryctl tax batch -operator <alice's back-office email>
   go run ./cmd/lotteryctl tax file -batch 7 > kra-withholding-batch-7.jsonl
   ```

   The file starts with a header line (format `kra-withholding-v1`, batch number, preparer, totals), then one event per
   line. `tax file` rebuilds it byte for byte from the database and refuses (`batch's events no longer hash`) if
   anything changed since it was prepared. Prepare the return for KRA from it; the header's `tax_kes` is the amount due.
2. **Pay KRA** for exactly the batch's `tax_kes`, and get the payment registration number (PRN).
3. **Record the remittance** (a **different** operator from the preparer):

   ```sh
   go run ./cmd/lotteryctl tax remit -batch 7 -prn 2026XXXXXXXX -amount 340 -operator <bob's back-office email>
   ```

   This is refused if the amount is not exactly the batch's tax, if the same person prepared the batch, if the PRN is
   already recorded, if the batch is already remitted, or if its events no longer hash to its file. It posts
   `TAX_PAYABLE → OPERATOR_BANK` and closes the batch for good.

`go run ./cmd/lotteryctl tax batches` lists the latest batches and their remittances.

## Deadlines and alerts

Withheld tax not yet remitted (unbatched, or in a batch without a PRN) raises `kra.remittance_due` (warning) once the
oldest event is 72 hours old and `kra.remittance_overdue` (page) at 120 hours (`KRA_REMIT_WARN_AFTER`,
`KRA_REMIT_DUE_AFTER`). These are **placeholders**: set them from the deadline a tax adviser confirms. Both clear when
the tax is remitted. See [alerts](alerts.md).

## Reconciliation

The daily payout reconciliation checks that `TAX_PAYABLE` equals the tax of live payouts minus everything remitted,
that each batch's totals are those of its events, that each remitted batch has exactly one remittance journal for its
tax (and unremitted ones none), and that each event matches its payout. A mismatch freezes payouts (see
[alerts](alerts.md)).

## Not yet settled (owner and tax adviser)

- How the return is filed (iTax upload, an API integration, or the adviser), and whether KRA wants this layout.
- Withholding certificates name the payee by KRA PIN or national ID. The platform stores only a **hash** of the
  national ID, so the file carries the player id. Decide whether to keep payees' ID numbers (encrypted) for this.
