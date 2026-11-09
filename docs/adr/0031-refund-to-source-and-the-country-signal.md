# 0031. Unpayable balances are refunded to source; the client's country is recorded, and with real money only Kenya is served

Status: accepted
Date: 2026-11-08

## Context
Two gaps remained in S3 (roadmap ID-04, ID-05):

- **Balances that cannot leave.** Money can only be withdrawn by an account that may be paid (ADR 0022, ADR 0030):
  active, identity verified, phone proved. An account that is suspended, whose identity was refused (an under-18
  among them), or that never verified, keeps its balance with no way out. `closureGuard` refuses to close an account
  while it holds money, so such an account can be neither paid nor closed.
- **No location signal.** LN 112 reg 82 asks a licensed operator to verify where a player is before taking money
  (`docs/compliance/requirements-matrix.md`). The platform knew nothing about where a request came from. The phone
  prefix proves nothing: Kenyan numbers roam.

## Decision
- **Refund to source.** An admin may refund the whole wallet balance of an account that may not be paid to the number
  that paid in its latest credited top-up (`payouts.RefundToSource`, `POST /v1/staff/players/{user}/refund`).
  - **What it is.** A withdrawal, through the existing payout machinery: the wallet is debited at once, the withdrawal
    tax rule in force applies, the payout is created `held`, and it is sent, verified, failed or settled like any other.
    The books and reconciliations need no change.
  - **What it records.** The reason and the requesting admin are stored on the payout (`refund_reason`,
    `refund_requested_by`, migration 00050). They are part of what the payout is: the transition trigger refuses to
    change them. The request is audited as `payout.refund.requested`, with why the account could not be paid.
  - **Two people.** It is approved by someone other than the admin who asked (`ErrSameApprover`, 409
    `two-person-rule`). It skips the payee re-check of ADR 0030, because the account cannot be paid by definition and
    the money goes to the payer, not to the account's number. Rejecting it gives the money back to the wallet.
  - **When it is refused:**
    - the account can withdraw itself (`payable`);
    - it owes money from a reversed top-up (`in-debt`);
    - the wallet is empty;
    - there is no credited top-up to send to (`no-source`).
- **The client's country.** `GEO_COUNTRY_HEADER` names the header the edge sets with the client's country
  (Cloudflare's `CF-IPCountry`). It is honoured only behind a trusted proxy (`TRUSTED_PROXY_HEADER`), which overwrites
  whatever the client sent.
  - **What is recorded.** Every sign-up, top-up, play and withdrawal writes the country to `compliance.geo_signals`
    (append-only, migration 00050), with whether the request was refused for it. An accepted sign-up's signal is
    written with the new account; a refused one has no account. The signals show on the player's timeline and in the
    player pack.
  - **Sandbox** records and refuses nothing.
  - **Live** refuses sign-ups, top-ups and plays from anywhere but `KE` with 451 `outside-kenya`: an unknown country
    (`XX`), Tor (`T1`) and a missing header are refused too. Withdrawals are never refused for where the player is:
    they pay the player's own money to their Kenyan M-Pesa number. `OPERATING_MODE=live` does not start without
    `GEO_COUNTRY_HEADER`.

## Consequences
- **Closing an account.** A suspended or unverifiable account with money can now be emptied and closed. Two staff
  members and the M-Pesa payer's number stand between a balance and its refund. The refund goes to whoever paid the
  latest top-up, which may not be the account holder (a shared phone). The admin checks this before asking.
- **For counsel:**
  - whether a refund to an underage account must also return the stakes it played;
  - whether the withdrawal tax applies to a refund;
  - what to do with a balance that has no credited top-up to refund to.

  The code refunds the balance only, taxed as a withdrawal, and refuses the last case
  (`docs/runbooks/payouts.md`).
- **What the country proves.** It is a signal from the client's IP address, not proof of location. A VPN changes it,
  and a Kenyan on a foreign network is refused until they switch. The regulator may ask for more (device location,
  KYC address). The signals table gives a record to show either way.
- **Cloudflare dependency.** The country comes from Cloudflare. If the edge changes (S12, hosting in Kenya), the new
  edge must set an equivalent header, or `live` will not start.
