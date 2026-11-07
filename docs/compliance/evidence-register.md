# Evidence register

Technical verification (`technically_verified`) is never regulatory approval (`externally_approved`). Only a document
from the regulator, a lab or a store moves a row to `externally_approved`, and the evidence column must name that
document.

Statuses: `not_started` · `in_progress` · `implemented` · `technically_verified` · `awaiting_external_evidence` ·
`externally_approved` · `blocked`.

| ID | Requirement (see requirements-matrix.md) | Implementation | Evidence | Owner | Status | External dependency |
|---|---|---|---|---|---|---|
| EV-01 | Licence held for the product and category (GCA-01, GCA-03) | Operating mode `live` gate (S1) | — | Operator | blocked | Licence from the GRA |
| EV-02 | Age verified before entry (GCA-04) | ID-01 (S3) | — | Engineering | not_started | — |
| EV-03 | Identity, age and location verified before activation and deposit (GCA-05) | ID-01, ID-05 (S3) | — | Engineering / Compliance | not_started | Counsel: is manual review acceptable? |
| EV-04 | National self-exclusion register integration (GCA-07) | — | — | Engineering | blocked | Register interface from the GRA |
| EV-05 | Operator self-exclusion, time-outs and deposit limits (GCA-08, part) | `internal/rg` | `internal/rg/rg_integration_test.go` | Engineering | technically_verified | — |
| EV-06 | Loss, session and spend limits; reality checks (GCA-08, rest) | — | — | Engineering | not_started | — |
| EV-07 | Daily entry cap (GCA-09) | `game.go:26`, DB constraint `00006` | `game_integration_test.go:231` | Engineering | technically_verified | Authority's cap value |
| EV-08 | Regulator API and central monitoring (GCA-11) | — | — | Engineering | blocked | Specification from the GRA |
| EV-09 | Player data in Kenya (GCA-12) | S12: move to hosting in Kenya (owner, 2026-11-07); provider being chosen | Render, Oregon today | Operator | in_progress | Kenyan VM hosting contract; data-centre location in writing |
| EV-10 | Tamper-evident audit log (GCA-13) | append-only `audit.audit_log` | `schema_integration_test.go` | Engineering | technically_verified | — |
| EV-11 | Geo-blocking (GCA-13) | ID-05 (S3) | — | Engineering | not_started | — |
| EV-12 | RNG certification (GCA-14) | `internal/game/fair`, `internal/draws`, `tools/verifier` | nightly distribution test | Operator | awaiting_external_evidence | Accredited lab |
| EV-13 | Charity share and prize economics (GCA-16) | prize shares are data (`00021`) | — | Operator / counsel | blocked | Counsel |
| EV-14 | Player funds segregation (GCA-17) | ledger `WALLET_LIABILITY` account | daily reconcile | Operator | awaiting_external_evidence | Bank account, auditor |
| EV-15 | M-Pesa approved as payment provider (GCA-18) | — | — | Operator | blocked | GRA |
| EV-16 | Winners' confidentiality (GCA-20) | certificates name no winners | draws tests | Engineering | technically_verified | — |
| EV-17 | Suspicious activity reporting within 24 h (GCA-21) | AML-01 (S5) | — | Compliance | not_started | MLRO appointed |
| EV-18 | ODPC registration (DPA-01) | — | — | Operator | not_started | ODPC |
| EV-19 | DPIA and retention policy (DPA-02) | ADR 0015 (proposed) | — | Operator / counsel | not_started | Counsel |
| EV-20 | Global operating gate (internal control) | S1 | — | Engineering | not_started | — |
| EV-21 | Callback cannot credit without verification (internal control) | S2 | — | Engineering | not_started | — |
| EV-22 | Google Play gambling application (GP-02) | — | — | Operator | blocked | Licence |
| EV-23 | Apple 5.3 review (AS-01) | — | — | Operator | blocked | Licence |
| EV-24 | Target API 36 (GP-03) | S6 | — | Engineering | not_started | — |
| EV-25 | Account deletion in app and on the web (GP-04, AS-03) | closure request; S8 | — | Engineering | in_progress | — |
