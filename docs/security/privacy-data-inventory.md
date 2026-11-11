# Privacy data inventory

What personal data the platform holds about players, where, why, and who else sees it. This is the source for the
privacy policy (`apps/mobile/web/privacy/`), the App Store privacy label and manifest
(`docs/mobile/privacy-manifests.md`) and the Play data safety form (S10). **For counsel to confirm.**

Retention periods are not decided. ADR 0015 (data retention) is proposed and waits for counsel on what the Data
Protection Act 2019, the Gambling Control Act and POCAMLA require. Until then nothing is deleted automatically, and a
closed account keeps its records.

## Player data

| Data | Where | Why | Notes |
|---|---|---|---|
| Phone number (M-Pesa) | `identity.users.msisdn` | Account, sign-in, top-ups and payouts | Proved by the first top-up |
| Email address | `identity.users.email` | Password reset links | |
| Password | `identity.users.password_hash` | Sign-in | Argon2id hash with a server pepper; never the password |
| First and second name, national ID number | `kyc.declared_identities` (sign-up), `kyc.kyc_checks` (verification) | Identity (ADR 0009, 0022) | Encrypted at rest (AES-GCM, `DATA_KEYS`); ID number also as a keyed hash for lookups |
| Date of birth, 18+ declaration | `kyc.age_declarations` | Age (ADR 0029) | Encrypted |
| Google/Apple account link | `identity.auth_identities` | Sign-in (ADR 0020) | The provider's subject id only |
| Google account photo link | `identity.users.photo_url` | Shown on the account (ADR 0035) | A link to Google's image host; the image is not stored |
| Devices and sessions | `identity.devices`, `identity.sessions`, `identity.refresh_tokens` | Device limit, attestation, signing out | Device fingerprint and tokens as hashes; platform; attestation verdict |
| Top-ups | `payments.payments`, `payments.c2b_*` | M-Pesa payments | Amounts, M-Pesa receipts, paying number, Daraja callbacks |
| Wallet and ledger | `wallet.*`, `ledger.*` | Balances, double-entry books | |
| Plays and draws | `tickets.tickets`, `game.spins`, draw entries | The game and its fairness proofs | |
| Payouts | `payouts.*` | Withdrawals, prizes, refunds; withholding tax | Receipts, numbers paid |
| Responsible-gambling limits, breaks, self-exclusions | `rg.*` | Player protection (LN 112) | |
| Account status history, closure requests | `identity.user_status_history`, `identity.closure_requests` | Audit (ADR 0030) | |
| Country of the connection | `compliance.geo_signals` | Kenya-only rule (ADR 0031) | From the IP address, via Cloudflare; the address itself is not stored |
| Monitoring cases | `compliance.aml_*` | Anti-money-laundering (ADR 0033) | Not shown to the player |
| Audit log | `audit.audit_log` | Who did what, for regulators and disputes | Append-only |
| On the phone | Secure storage (tokens), shared preferences (settings) | Staying signed in | Not backed up (S6) |

## Who else receives it (processors and recipients)

| Recipient | What | Why | Where |
|---|---|---|---|
| Safaricom (M-Pesa Daraja) | Phone number, amounts, receipts | Payments | Kenya |
| Render (hosting) | Everything above (database, api, workers) | Hosting | **United States (Oregon)**: a cross-border transfer; S12 moves hosting to Kenya (LN 112 reg 42) |
| Cloudflare | Connection metadata (IP, country) | TLS, protection | Global edge |
| Google, Apple | The sign-in exchange | Sign in with Google or Apple | |
| The SMTP provider (`SMTP_HOST`) | Email address, reset links | Password resets, staff notices | Whichever the owner configures |
| Staff (back office) | What their role needs; identity documents are audited when viewed | Operations, KYC review | |
| Regulators, the FRC, KRA | As the law requires | Licensing, AML reports, tax | |

No advertising, analytics or tracking SDK is used.

## Players' rights (Data Protection Act 2019)

- **Access:** players can request a copy of their data; the back office's player pack (CSV) is the export.
- **Correction:** of names and ID number, through support. Declared identities are append-only, so a correction is
  a new verified submission.
- **Deletion:** the account closes on request (in the app, or `/delete-account/`). Records the law requires are kept
  for their periods, then pseudonymised (ADR 0015, pending).
- **Objection and complaints:** to the operator first, then the Office of the Data Protection Commissioner.

Open items for counsel are listed in the drafts themselves (`[TO CONFIRM]`).
