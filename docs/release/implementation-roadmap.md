# Implementation roadmap (compliance-first, licensing pending)

Owner decisions of 2026-11-07:

- **Operating mode** is `disabled | sandbox | live`. The deployed service may run `sandbox` only while every Daraja URL
  is the sandbox. `live` needs recorded licence and approval evidence. Unknown or inconsistent configuration means
  `disabled`.
- **Age:** a server-side 18+ declaration with date of birth at sign-up in every mode. In `live`, verified ID is also
  required before any top-up or play. ADR 0022 stays in force for `sandbox`.
- **Delivery:** one branch and PR per slice, reviewed by the owner before the next one starts.

Technical completion never means legal authorisation. Milestone 7 (public real-money launch) is **BLOCKED** until the
licence and the store approvals exist (`docs/compliance/evidence-register.md`).

## Baseline

See `docs/architecture/repository-audit.md` § 8. Go build, lint and unit tests pass. Integration, e2e and Flutter
were not runnable on the audit machine (no Docker daemon, no Flutter SDK); CI runs them on every PR.

## Backlog

Priority: P0 blocks any real-money operation or is an exploitable money defect; P1 is needed for licensing or store
submission; P2 hardens.

| ID | P | Slice | Problem | Acceptance | Depends on | Status |
|---|---|---|---|---|---|---|
| REG-01 | P0 | S1 | No global operating gate; plays cannot be stopped | Every money path and job refuses in `disabled`; `live` refuses without two staff approvals and licence evidence; `sandbox` refuses with production Daraja URLs; mode changes are audited and alerted | — | not_started |
| PAY-01 | P0 | S2 | Late/orphan callbacks credited without M-Pesa verification (SEC-01) | A callback alone never credits; verified by Transaction Status or held for review | — | done (ADR 0028) |
| PAY-02 | P0 | S2 | Callback amount trusted and overwritable (SEC-02) | First callback is kept; amount ≠ requested goes to review | — | done (ADR 0028) |
| PAY-03 | P1 | S2 | Hook secret in traces and logs (SEC-03) | Span names and logs redact the segment | — | done (ADR 0028) |
| PAY-04 | P1 | S2 | No DB state machine for top-ups (SEC-04) | Illegal transitions rejected by a trigger | — | done (ADR 0028) |
| ID-01 | P0 | S3 | No server-side age evidence before money in | Sign-up requires date of birth and declaration; under 18 refused; `live` needs verified ID before top-up and play | REG-01 | done (S3a, ADR 0029) |
| ID-02 | P1 | S3 | Payouts and pushes don't re-check eligibility | `Send`, `Approve` and the push job re-check | — | done (S3b, ADR 0030) |
| ID-03 | P1 | S3 | No status history; self-exclusion keeps sessions | Append-only history; exclusion revokes sessions | — | done (S3b, ADR 0030) |
| ID-04 | P1 | S3 | Balances that can't be paid have no exit | Staff refund-to-source with reason and audit | — | done (S3c, ADR 0031) |
| ID-05 | P1 | S3 | No jurisdiction signal | Country recorded; non-KE refused in `live` | REG-01 | done (S3c, ADR 0031) |
| OPS-01 | P1 | S4 | Single-person payout and settle powers; CLI official spoofing | Second person enforced in the API and CLI | — | done (ADR 0032) |
| AML-01 | P1 | S5 | No transaction monitoring or cases | Configurable rules raise cases; holds honoured | ID-02 | done (ADR 0033; thresholds pending the compliance owner) |
| AND-01 | P0 (store) | S6 | Release signed with debug key; versionCode fixed | Release fails without upload key; build number from CI | — | done (`docs/mobile/android-release.md`; the upload key is the owner's) |
| IOS-01 | P0 (store) | S7 | No privacy manifest, URL types, export flag | Present and correct | — | done (`docs/mobile/ios-release.md`; team and Google client are the owner's) |
| WEB-01 | P0 (store) | S8 | No privacy, terms, RG, deletion pages; no in-app links | Pages published (DRAFT for counsel); linked in the app | — | not_started |
| CI-01 | P2 | S9 | No Flutter/npm dependency scan; dead `web` job | osv-scanner on lockfiles; job fixed | — | not_started |
| RG-01 | P1 | S11 | LN 112 reg 84 requires loss, session and spend limits and reality checks; only deposit limits and time-outs exist | Limits enforced server-side on play; reality check served by the API and shown by the app | — | not_started |
| REG-02 | P0 (launch) | S12 | LN 112 reg 42: player data must be stored in Kenya unless exempted; production runs on Render, Oregon. **Owner, 2026-11-07: hosting in Kenya** (virtual machines; cPanel packages cannot run it) | Database, cache, api, workers, backups and logs with player data all in Kenya; encrypted at rest; Render retired; location evidenced in writing | Safaricom account and product details | in_progress |
| REG-03 | P1 | — | LN 112 regs 41(e), 44, 73: regulator API, central monitoring, national self-exclusion register | Adapter built against the GRA's published specification | GRA specification | blocked |
| STO-01 | P1 | S10 | No store submission pack | Checklists, reviewer notes, device matrix | AND-01, IOS-01, WEB-01 | not_started |

### S12: Move to hosting in Kenya (owner decision, 2026-11-07)

LN 112 reg 42 requires player data to be stored in Kenya. Everything that holds player data moves: PostgreSQL, Valkey,
the api and its workers, backups, and any logs or traces that carry personal data. The static sites (player web app,
back office) hold no player data; they may move too for simplicity.

What carries over unchanged: the api, worker and lotteryctl container images (`images.yml`, signed with cosign),
migrations on start, the operating mode (ADR 0027), the health probes.

What has to be decided or built:

| Item | Today (Render) | Needed on Safaricom hosting |
|---|---|---|
| Compute for api and workers | Render Docker service, workers in-process | VMs or containers in a Kenyan data centre; a separate worker process if affordable |
| PostgreSQL | Render Postgres 18, single-role mode | Managed Postgres or self-run on a VM; the four app roles of `00008` (ends single-role mode, SEC-08); encryption at rest; point-in-time backups kept in Kenya |
| Valkey | Render Key Value | Managed or self-run Valkey (rate limits, attestation challenges) |
| TLS and client address | Cloudflare in front, `CF-Connecting-IP` | Whatever fronts the service; `TRUSTED_PROXY_HEADER` re-measured (`docs/runbooks/trusted-proxy.md`) |
| M-Pesa hooks | Secret path only (ADR 0028) | Optionally an allowlist of Safaricom's published callback addresses, once the client address is trustworthy on the new host (deferred from S2, SEC-03) |
| Deploys | Render Blueprint, `deploy-prod.yml` | A deploy workflow for the new host behind the `production` environment |
| Secrets | Render env vars | The host's secret store; `DATA_KEY`, `IDENTITY_KEY`, `AUTH_KEY` carried over unchanged |
| Data | Render database | `pg_dump`/restore during a short maintenance window with operations stopped (ADR 0027), verified by the reconciliations |
| DNS | Safaricom DNS, records to Render | Records to the new host (`api.`, `backoffice.`, apex) |
| Evidence | — | Contract or letter naming the data-centre location, for the evidence register (EV-09) |

Safaricom's cPanel hosting packages cannot run this (no long-running processes, no PostgreSQL roles or triggers, no
Valkey, no containers). What is needed is virtual machines with root access in a Kenyan data centre: to start, one of
about 4 vCPU, 8 GB RAM and 100 GB SSD on Ubuntu with a static IP and backups kept in Kenya, ideally a second for the
database. First ask Safaricom Business for cloud virtual machines (its VMware-based cloud in Nairobi and Kisumu);
fallback Angani or Lineserve (Nairobi). Get the data-centre location in writing.

Needed from the owner: the provider and product chosen, account access for deployment, and whether the free-database
rotation (`db-rotate`) can be retired once Render goes.

### Later backlog (not scheduled)

- Regulator reporting adapter (needs the regulator's specification; adapter interface only until then).
- Data retention and pseudonymisation on closure (ADR 0015 is proposed; needs counsel on retention periods).
- Spin seed rotation order (SEC-09); locked debt read (SEC-10).
- Native App Attest and Play Integrity; push notifications.
- Least-privilege DB roles on a paid database (SEC-08).

## External dependencies

| Dependency | Owner | Blocks |
|---|---|---|
| Licence category, number and conditions | Operator, counsel, regulator | `live` mode, store gambling applications, Milestone 7 |
| Confirmation of price (KES 1), odds, prize shares, excise and withholding | Counsel, tax adviser | Launch price version |
| Whether manual ID review and STK phone proof are acceptable | Regulator | KYC vendor choice |
| Regulator monitoring/reporting specification | Regulator | Reporting adapter |
| RNG certification | Accredited lab | Licence |
| Daraja production credentials and paybill | Safaricom | `live` |
| Play Console, upload key | Owner | Android store submission |
| Apple Developer team, certificates | Owner | iOS archive and submission |
| Firebase, Play Integrity and App Attest projects | Owner | Push, attestation |
| Physical devices | Owner / QA | Device matrix |
