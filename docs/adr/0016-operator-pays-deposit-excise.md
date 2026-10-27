# 0016. The operator pays the deposit excise; players are taxed only on winnings

Status: accepted
Date: 2026-10-26

## Context
Since 2026-10-22 the 5% excise duty on deposits (`deposit_excise`) was deducted from each top-up: a KES 100 top-up
credited KES 95 and showed the excise as its own wallet line (ADR 0006). The owner decided (2026-10-26) that a
player should not be taxed when topping up to play, only when winnings are paid out. The excise itself is still owed
to KRA on every deposit; who bears it is the operator's choice.

## Decision
- A confirmed top-up (STK, paybill, a resolved unconfirmed payment or suspense) credits the **whole amount** to the
  wallet. The excise at the rate version in force is posted from `EXCISE_EXPENSE` to `EXCISE_PAYABLE`: the operator
  still owes and remits it, from its own funds (`ledger.Deposit` with `deductFromCredit = false`, a path that already
  existed). No `deposit_excise` wallet line is written for new deposits; older ones keep theirs.
- Winnings keep their withholding tax (`winnings_wht`, 20% placeholder), taken when the prize is paid and before it
  is sent to M-Pesa. Withdrawing the wallet stays untaxed (`withdrawal_wht` 0%): it is the player's own deposits.
- A top-up is still refused (503) when no excise version is in force: the operator's liability must be computed.
- A reversed deposit takes back what the player received (the whole deposit); the excise booked on it stays an
  operator cost, as before.

## Consequences
- The operator's cost rises by 5% of deposits (`EXCISE_EXPENSE`); players' balances and limits count whole shillings
  deposited.
- Player apps no longer mention excise on top-up; the history still labels `deposit_excise` lines from before.
- To be confirmed with the tax adviser together with the excise version itself (STATUS, open questions).
