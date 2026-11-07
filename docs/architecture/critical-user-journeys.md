# Critical user journeys (baseline, 2026-11-07)

Each journey runs from the Flutter screen through the API route (`internal/api/api.go`, all under `/v1`) to the module
and its tables. "Gap" marks what the journey lacks today; the fixes are planned in
`docs/release/implementation-roadmap.md`.

| # | Journey | Screen | Route | Service | Tables | Money effect | Tests | Gap |
|---|---|---|---|---|---|---|---|---|
| 1 | Account creation | `features/auth/.../sign_in_screen.dart` (create mode) | `POST /auth/signup`, `POST /auth/sso/{provider}/signup` | `identity` | `identity.users`, `kyc.declared_identities` | none | `identity_integration_test.go` | 18+ is a client-only checkbox (`sign_in_screen.dart:60`); no date of birth; no country signal |
| 2 | Authentication | sign-in | `POST /auth/login`, `/auth/refresh`, `/auth/sso/{provider}` | `identity/session.go` | `identity.sessions` | none | `identity_integration_test.go:108,144,482` | `Authenticate` checks the session, not `users.status` (`session.go:315-342`); covered by revocation on suspend/close |
| 3 | Age and identity verification | `account/.../kyc` | `POST /kyc`, `GET /kyc` | `kyc.Submit` (`kyc.go:139-177`) | `kyc.kyc_checks` (append-only) | none | `kyc_integration_test.go:188,669` | Date of birth optional under manual review yet `is_adult=true` |
| 4 | Eligibility | — | — | `identity/eligibility.go:24-106`, `rg.AssertNotRestricted` | `identity.users`, `rg.restrictions` | — | see compliance gap analysis § 2 | No global gate; no geo; no ID before money in |
| 5 | STK top-up start | `wallet/.../top_up` | `POST /payments/topups` | `payments.StartTopUp` (`payments.go:235-330`) | `payments.payments` | none until credit | `payments_integration_test.go:315` | The push job doesn't re-check RG (`payments.go:380`) |
| 6 | STK callback | — | `POST /hooks/mpesa/stk/{path}` | `payments.HandleCallback` (`payments.go:436-466`) | `payments.payments.raw_callback`, `orphan_callbacks` | verify job enqueued | `payments_integration_test.go:427,500` | Late callback credits without verification; callback amount trusted |
| 7 | Wallet credit | — | — (job) | `payments.Verify` → `creditPayment` → `wallet.Apply` | `wallet.*`, `ledger.*` | +balance, deposit journal | `wallet_integration_test.go:148,182,276` | — |
| 8 | Paid play | `play/presentation/play_screen.dart` | `POST /plays` | `game.Service.Play` (`game.go:130-225`) | `tickets`, `game.spins` | −price, stake journal | `game_integration_test.go:231,309,402,427` | "Pay with M-Pesa and spin" is client-driven (`pay_and_play.dart:33-64`) |
| 9 | Outcome generation | wheel animates server slot | (in `POST /plays`) | `game/fair` (`fair.go:46-97`) | `game.chains`, `game.spins` | none | `fair` tests, nightly 10M-spin test | Seed rotation creates the new server seed after the client seed is received (`game.go:440`) |
| 10 | Draw execution and approval | back office Draws | `/v1/staff/draws/...` | `draws` (`draws.go:155-740`) | `draws.*` (append-only) | prize pool journals | draws integration tests | `lotteryctl` takes a free-text official (`cmd/lotteryctl/main.go:486`) |
| 11 | Prize allocation | Draws tab | — (job `payouts.prizes`) | `payouts/prizes.go:75-105` | `payouts.payouts` | prize journal, WHT | payouts tests | — |
| 12 | Withdrawal | `wallet/.../withdraw` | `POST /payouts/withdrawals` | `payouts.RequestWithdrawal` (`payouts.go:241-317`) | `payouts.payouts` | −balance held | `payouts_integration_test.go:379,515` | Debt read without a lock (`payouts.go:272`) |
| 13 | Payout processing | — | B2C hooks | `payouts/send.go` | `payouts.payouts` (DB state machine `00023`) | B2C | `TestWithdrawal_FaultMatrix` | `Send`/`Approve` don't re-check eligibility; one-person approve/settle |
| 14 | Transaction history | Wallet tab | `GET /payments`, `GET /payouts`, `GET /wallet` | payments, payouts, wallet | — | — | app_flows_test | — |
| 15 | Suspension | back office Players | staff route | `identity.Suspend` (`account.go:221`) | `identity.users`, audit log | none | `staff_integration_test.go:467` | No status history table |
| 16 | Self-exclusion | `account/.../responsible_gaming_screen.dart` | `POST /rg/self-exclusion` | `rg.go:175` | `rg.restrictions` (append-only) | blocks money in and play | `rg_integration_test.go` | Sessions stay alive; staff cannot set it |
| 17 | Account deletion | `account/.../closure_screen.dart` | `POST /account/closure` → staff close | `identity.RequestClosure`/`CloseAccount` (`account.go:46,104`) | `identity.closure_requests` | must be zero balance (`staff.go:1044`) | `account_integration_test.go:14` | Nothing deleted; no exit for balances that can't be paid; no web deletion page |
| 18 | Administrative review | back office | `/v1/staff/*` | `staff`, `oversight` | audit log | — | `staff_integration_test.go` | Role check in API; no dual control on payouts |
