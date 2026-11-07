# Requirements matrix (Kenya), researched 2026-11-07

**Not legal advice.** Every row needs confirmation by Kenyan counsel before it drives a product decision. Basis tags:
**P** = read in a primary source; **P-s** = an official page seen only through search snippets; **S** = secondary only
(law firms or press); **U** = not verified.

Main primary source: **LN 112/2026, Gambling Control (Conduct of Gambling Operations) Regulations, 2026**, Kenya
Gazette Supplement No. 163, 29 June 2026, read from the Parliament library scan
(https://libraryir.parliament.go.ke/bitstreams/99a775a9-cebc-4d55-b20d-97655ab9f1ce/download) through OCR. Quote the
official copy, not this summary. The text of the Gambling Control Act 2025 (No. 14 of 2025) could not be loaded from
Kenya Law, so **its section numbers below come from secondary sources.**

Kind: **M** = mandatory by law; **L** = depends on licence category; **ST** = store policy; **R** = recommended.

## Gambling law

| ID | Requirement | Source | Basis | Kind | Technical control | Status |
|---|---|---|---|---|---|---|
| GCA-01 | Running, hosting or providing payment services for an unlicensed gambling platform is an offence | LN 112 reg 102 | P | M | Operating mode: `live` needs licence evidence (S1) | planned |
| GCA-02 | The Act (assented 7 Aug 2025; commencement 20 or 26 Aug 2025, disputed) creates the Gambling Regulatory Authority, which replaces BCLB | Act No. 14 of 2025; CM Advocates | P-s / S | M | — | counsel |
| GCA-03 | Licence categories: public lottery (online among them, LN 111 reg 10); online gambling, lottery included (reg 11); platform provider (reg 15). LN 111 is partly stayed by the High Court | LN 111 via CM Advocates, iGaming Business | S | L | — | counsel |
| GCA-04 | No under-18s may play; age checked before entry is accepted | LN 112 regs 91(1)(a), 92 | P | M | ID-01 (S3) | planned |
| GCA-05 | Identity, location and age verified against reliable, independent, electronically verifiable sources **before account activation and before any deposit**; every winner verified before payout | LN 112 reg 82 | P | M | `live`: verified ID before top-up and play (S3); manual review may not count as "electronically verifiable" → registry vendor | planned / counsel |
| GCA-06 | POCAMLA KYC for every punter; no anonymous play; no virtual assets | LN 112 regs 83, 95(2) | P | M | KYC; M-Pesa only | partial |
| GCA-07 | National self-exclusion register run by the GRA. Integrate within 6 months, block automatically, check daily; refund deposits within 7 days of exclusion; report attempts | LN 112 regs 68–75 | P | M | Adapter once the register's interface exists | blocked (regulator) |
| GCA-08 | In-app player tools: deposit limits (daily/weekly/monthly); loss, session and spend limits; reality checks; self-exclusion of at least 24 h | LN 112 reg 84 | P | M | Deposit limits and time-outs exist; **loss, session and spend limits and reality checks are missing** | gap |
| GCA-09 | The Authority may cap entries per participant; excess entries are void | LN 112 reg 86 | P | L | Daily cap of 4 exists (`game.go:26`) | implemented |
| GCA-10 | Minimum stake (a KES 20 figure is reported) | Nation 2023 (proposal); Afriwise | S | U | Price is versioned data (`game.odds_config`), so changing it needs no code | counsel |
| GCA-11 | Secure API access for the Authority; integration with the central monitoring system and national gambling register | LN 112 regs 41(e), 44 | P | M | Reporting adapter (later); no specification yet | blocked (regulator) |
| GCA-12 | Player data encrypted in transit and at rest and **stored in Kenya** unless the Authority exempts it in writing | LN 112 reg 42 | P | M | Production runs on Render, Oregon (USA). **Owner, 2026-11-07: moving to Safaricom hosting in Kenya** (S12) | planned |
| GCA-13 | Encrypted, tamper-evident audit log; geo-blocking | LN 112 reg 41 | P | M | Append-only audit log exists; geo-blocking is ID-05 (S3) | partial |
| GCA-14 | RNG certified by the GRA or an approved lab; return-to-player disclosed | LN 112 reg 45 | P | M | Provably fair design; no certification | blocked (lab) |
| GCA-15 | Lottery systems from a licensed, approved provider | LN 112 reg 26(2)(c) | P | L | — | counsel |
| GCA-16 | **Public lottery: 30–45% of gross proceeds to charity** | LN 112 reg 27 | P | L | Prize shares are data (`00021`); the economics need modelling | counsel (critical) |
| GCA-17 | Player funds kept separate from operating funds; "saving component" (meaning unclear); lottery account at a CBK-licensed bank | LN 112 reg 41(f)–(g), reg 30 | P | M | Ledger separates player liability; banking is the owner's | counsel |
| GCA-18 | Payment providers (M-Pesa) need GRA approval; PCI-DSS gateway | LN 112 regs 30, 85 | P | M | — | owner |
| GCA-19 | Payouts up to KES 500k automatically to the mobile wallet; KES 500k–5M within 5 working days with ID check; late payment penalties after 14 days | LN 112 Third Schedule, reg 101 | P | M | Current manual-approval threshold is KES 100k (stricter); check the late-payment timers | review |
| GCA-20 | Winners' identities confidential without written consent | LN 112 reg 87 | P | M | Certificates name no winners (STATUS 2026-10-25) | implemented |
| GCA-21 | Fraud and suspicious activity reported within 24 h; records of tickets, transactions, accounts and complaints | LN 112 regs 88, 90 | P | M | AML-01 (S5); complaints log missing | gap |
| GCA-22 | Advertising: prior GRA approval, classification, RG messages, nothing aimed at minors | LN 114 via Afriwise | S | M | No marketing channel today | n/a yet |
| GCA-23 | At least 30% Kenyan shareholding; customer care in Kenya; funds in a local bank | Act via Afriwise | S | M | — | owner |
| TAX-01 | Lottery excise 5% of ticket price; betting/gaming excise 5% of deposits; 5% withholding on withdrawals replaces 20% on winnings (applicability to lotteries unclear) | Finance Act 2025 via EY | S | M | Tax rules are versioned data; current setup (5% deposit excise, 20% winnings WHT) may be the wrong pair | counsel / tax |

## Data protection

| ID | Requirement | Source | Basis | Kind | Control | Status |
|---|---|---|---|---|---|---|
| DPA-01 | Gambling operators must register with the ODPC whatever their size | Registration Regs 2021, Third Schedule item 3 | P | M | — | owner |
| DPA-02 | DPIA for high-risk processing; retention no longer than necessary; cross-border transfer conditions | DPA 2019 ss.31, 39, 48–50 | U | M | Data inventory (S8); retention (ADR 0015) | gap |

## AML

| ID | Requirement | Source | Basis | Kind | Control | Status |
|---|---|---|---|---|---|---|
| AML-R1 | Casinos (including internet casinos) are POCAMLA reporting institutions; whether online lotteries are is **unconfirmed** | POCAMLA s.2 via CM Advocates | S | L | — | counsel |
| AML-R2 | If covered: FRC registration, CDD and EDD, STRs within 2 days, records kept 7 years, an MLRO | POCAMLA ss.44–47A | S | L | Case management (S5); retention ≥ 7 years for financial records | planned |

## Store policy

| ID | Requirement | Source | Basis | Control | Status |
|---|---|---|---|---|---|
| GP-01 | Kenya is "allowed with limitations" for lotteries | support.google.com/googleplay/android-developer/answer/12256011 | P | — | — |
| GP-02 | Gambling application approved before release; licensed operator; geo-block; age gate; free install; no Play Billing; responsible-gambling info in the app and the listing | answer/9877032 | P | S8, S10 | planned |
| GP-03 | Target API 36 for new apps and updates from 31 Aug 2026 | developer.android.com target-sdk | P-s | S6 | verify |
| GP-04 | In-app deletion and a web deletion URL; data kept for regulatory reasons disclosed | answer/13327111 | P | S8 | planned |
| AS-01 | 5.3.4: licensed, geo-restricted, free; 5.3.3: no IAP for credit | App Review Guidelines | P | S10 | planned |
| AS-02 | Submitted by the legal entity (5.1.1(ix)) | App Review Guidelines | U | — | owner |
| AS-03 | 5.1.1(v): in-app account deletion | App Review Guidelines | P | S8 | partial |
| AS-04 | Privacy manifest with required-reason APIs | developer.apple.com news | P-s | S7 | planned |

## Questions for counsel

1. Which licence fits: online public lottery (LN 111 reg 10) or online gambling–lottery (reg 11)? Is the lucky spin a separate game of chance?
2. Does the 30–45% charity share (LN 112 reg 27) apply? What is the "saving component" (reg 41(g))?
3. Does any minimum stake apply to a KES 1 entry? Cite the section.
4. What is the state of the LN 111 litigation and of the fee and capital suspension?
5. Kenyan hosting or a reg 42(2) exemption? Is there a monitoring API specification and an onboarding process?
6. Is manual ID review "electronically verifiable" under reg 82, or is a registry check (IPRS) required?
7. Are online lottery operators POCAMLA reporting institutions?
8. Which excise and withholding apply to KES 1 lottery entries and to prizes paid to M-Pesa?
9. Does M-Pesa need GRA approval as a payment provider (reg 30(3))?
10. Confirm the Act's commencement date and its sections on age, advertising and shareholding from the Gazette copy.
