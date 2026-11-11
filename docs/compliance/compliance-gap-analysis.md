# Compliance gap analysis (baseline, 2026-11-07)

This is a technical analysis, not legal advice. Which obligations apply depends on the licence category that
4draws obtains, which is undecided. Requirements and their sources are in `requirements-matrix.md`; the evidence
status of each is in `evidence-register.md`.

## 1. Operating state

There is no switch between demonstration and real-money operation. Money paths are on whenever Daraja credentials are
configured (`internal/platform/config/payments.go:15-131`); plays have no switch at all (`internal/app/app.go:214`).
The deployed service runs the Daraja sandbox only, so no real money moves today, but nothing in the code would stop
production credentials from enabling real-money play without a licence. **Planned:** S1, operating mode.

## 2. Player eligibility (implemented and tested)

`internal/identity/eligibility.go` holds the asserts, re-read inside each money transaction:

| Path | Checks | Test |
|---|---|---|
| STK top-up (`payments.go:257-277`) | active, RG restriction, cooldown, deposit limit | `payments_integration_test.go:315` |
| C2B validate and settle (`c2b.go:136-180`) | proved phone, active, RG, bounds, limit | `c2b_integration_test.go:380,474` |
| Play (`game.go:148-173`) | active, proved phone, RG, no debt, 4 a day | `game_integration_test.go:309` |
| Withdrawal (`payouts.go:241-283`) | active, verified ID, proved phone, no debt, 3 a day | `payouts_integration_test.go:379` |
| Prize (`prizes.go:75`) | as withdrawal; otherwise owed | payouts tests |

Gaps: payout `Send`/`Approve` and the STK push job do not re-check; no global gate; no jurisdiction.

## 3. Age and identity

- The 18+ confirmation is a client checkbox, not sent to the server (`apps/mobile/lib/features/auth/presentation/sign_in_screen.dart:60,94,199`; `internal/api/auth.go:95-102`).
- Date of birth is checked only at ID submission, and is optional under manual review while the row is stored with
  `is_adult=true` (`internal/kyc/kyc.go:139-177`).
- Under ADR 0022 a player may top up and play before any ID review: "an under-18 can play but can never be paid".
  A refused minor's balance can be neither paid nor closed (`internal/api/staff.go:1044-1052`).

**Planned:** S3 (ID-01, ID-04).

## 4. Jurisdiction

Only Kenyan mobile numbers are accepted (`internal/identity/msisdn.go:12-37`, DB check `00003:4`), and M-Pesa needs a
Kenyan SIM. Neither proves location. No IP-country or device location is used. **Planned:** S3 (ID-05).

## 5. Responsible gambling

Implemented and tested: deposit limits with a 24 h cooling-off on increases, time-outs (24 h/7 d/30 d), self-exclusion
(6 months/1 year/permanent), append-only and never shortened (`internal/rg/rg.go`). Enforced on every money-in and
play path.

Missing: loss, session and spend limits and reality checks, all required by LN 112 reg 84; staff-imposed
restrictions; session revocation on self-exclusion; integration with the GRA's national self-exclusion register
(LN 112 regs 68–75, awaiting its interface).

## 5a. Hosting

LN 112 reg 42 requires player data to be stored in Kenya unless the Authority exempts it in writing. Production runs on
Render in Oregon, USA (`docs/STATUS.md`). This blocks `live` operation; it does not block sandbox demonstration.
**Owner, 2026-11-07:** the platform moves to hosting in Kenya on virtual machines (slice S12 in the roadmap).

## 6. AML

None: no monitoring, risk scoring, case management, sanctions screening or suspicious-transaction reporting workflow.
Operational limits exist (top-up cooldown, withdrawal caps, manual approval above KES 100,000). **Planned:** S5,
with every threshold configurable and owned by a compliance officer.

## 7. Draw integrity

Strong technical foundation: drand quicknet with pinned key, committed seeds, append-only evidence, two officials,
independent verifier, daily reconciliation. Not a certification: no accredited RNG laboratory has tested it.
Gap: the CLI path lets one person act as both officials (S4).

## 8. Data protection

No retention or deletion is implemented (ADR 0015 proposed). Closure keeps all personal data. No privacy notice is
published. ODPC registration status of the operator is unknown (owner).

## 9. Regulator reporting

None. No regulator specification is available; an adapter will be built only against a published contract.

## 10. Product classification (for counsel)

- A play costs KES 1 (`db/migrations/00036_play_price_1.sql`), set for sandbox testing; the launch price is
  undecided. Any statutory minimum stake must be confirmed against the licence category.
- A spin has no cash prize; it adds draw chances. Prizes come from draws funded by a share of stakes. Whether this is a
  lottery, an online game of chance or a prize competition decides the licence.
- Odds, chance tiers, prize shares, the 5% deposit excise (operator-paid) and 20% winnings withholding are placeholders
  (`docs/STATUS.md`).
