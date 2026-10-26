# Runbook: the back office

The back office (`apps/backoffice`) is how staff run the operations that `lotteryctl` performs (ADR 0013). Every
action is checked against the staff member's role by the API and audited as `staff:<id>`. `lotteryctl` still works for
emergencies (audited as `operator:<name>`).

## Sign-in and roles

Staff sign in through the staff realm (Keycloak). An administrator gives each person only the roles their job needs:

| Role | Can |
|---|---|
| `kyc_reviewer` | see submitted identity details (every view is audited as unmasked), approve or reject them with a reason |
| `payments_operator` | run the payments and game reconciliations, record reversals, resolve unconfirmed top-ups, credit or refund suspense payments, re-check paybill payments |
| `draw_official` | sign frozen draws, approve conducted draws, void a draw before its round (with a reason) |
| `payout_operator` | approve or reject held payouts, settle manual payouts from the statement, retry failed prizes |
| `tax_officer` | prepare KRA batches, download batch files, record remittances |
| `alerts_operator` | acknowledge alerts (lifts a payout freeze caused by a failed reconciliation) |
| `support` | close accounts whose holders asked |
| `auditor` | read every list and run reconciliations; changes nothing |

**Two-person rules hold in the back office as in `lotteryctl`:** two different officials sign, and two approve, each
draw; the person who records a KRA remittance must not be the one who prepared the batch. Give `draw_official` and
`tax_officer` to at least two people each.

## Configuration (API)

```
STAFF_OIDC_ISSUER=https://<keycloak>/realms/lottery-staff
STAFF_OIDC_AUDIENCE=lottery-backoffice-api
STAFF_OIDC_JWKS_URL=https://<keycloak>/realms/lottery-staff/protocol/openid-connect/certs
STAFF_DATABASE_URL=postgres://<worker login>@…      # the worker's role; omit only in single-role mode
CORS_ALLOWED_ORIGINS=https://<back-office origin>,https://<player web origin>
```

The realm needs: realm roles named as above; a public client for the back office with Authorization Code + PKCE
(S256), its redirect URI and web origin; an audience mapper adding `lottery-backoffice-api` to access tokens;
**MFA required for every staff account**; short access tokens (10 min). `deploy/keycloak/lottery-staff-realm.json`
is the local version (test staff `alice`, `bob`, `kim`, `paula`, `otis`, `tara`, `tom`, `sam`, password
`local-dev-staff`; never import it anywhere real).

## Local

`make dev` starts Keycloak with the staff realm and an api that accepts it. Then:

```sh
cd apps/backoffice && cp .env.example .env.local && npm ci && npm run dev    # http://localhost:5173
```

## Daily operations

- **Identity review:** open *Identity review*, compare each submission with the ID, approve or reject with what you
  checked. Opening the list is audited.
- **Reconciliations:** run them from *Payments* and *Overview* when an alert says one failed or is stale; the run is
  stored. Mismatches stay listed until fixed.
- **Draws:** sign each frozen draw before its drand round (09:00 Nairobi); approve conducted draws. *Late* steps are
  shown at the top.
- **Payouts:** approve or reject *Held for approval*; settle *Needs manual settlement* from the M-Pesa statement.
- **Tax:** prepare a batch, file it with KRA, then a second officer records the remittance with its PRN.
- **Closures:** close accounts on request once the balance is paid out; the API refuses while money is in the
  account or moving and says why.

The draw entry list (which ties entries to players) is not in the back office: it is for the regulator
(`lotteryctl draws entries`).
