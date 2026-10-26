# Wallet — page rules

- The balance is the server's (`GET /v1/wallet`), labelled "Confirmed balance". It never moves because the player
  came back from M-Pesa, because time passed, or because a prompt was sent: only a server answer moves it. It
  crossfades to a new value; it never counts up.
- Top-ups and withdrawals follow `SettlementFlow`: send once with one idempotency key (a lost answer is retried with
  the same key) → pending (blue, "Pending M-Pesa confirmation") → the server's final state. Status checks back off
  (2, 3, 5, 8, 13, 15 s …) and stop after 3 minutes with "safe to leave"; the server's list shows the item when the
  player returns.
- While a top-up is pending on the server, a new one cannot start ("To avoid paying twice…").
- The M-Pesa prompt always goes to the account's own number, shown before confirming. The excise is described, not
  computed: the server credits the net amount.
- `confirmed` haptic once when a top-up is credited or a payout paid; never for pending.
