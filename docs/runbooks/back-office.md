# Runbook: the back office

The back office (`apps/backoffice`) is how staff run the operations that `lotteryctl` performs (ADR 0013, 0018). Every
action is checked against the staff member's role by the API and audited as `staff:<id>`. `lotteryctl` still works for
emergencies (`-operator <your staff id>`, audited as `staff:<id>` like the back office).

## Sign-in and roles

Staff sign in with **Google or Apple** (ADR 0018). Nobody can sign in uninvited: an admin invites each person on the
*Staff* page by the email address of their Google or Apple account and gives them only the roles their job needs.
The first sign-in binds that account to the member; roles and disabling apply from the next request.

| Role | Can |
|---|---|
| `admin` | invite staff, assign roles, disable and re-enable members (never their own) |
| `finance` | read the *Finance* dashboard: deposits, stakes, prizes, withdrawals, revenue, what is owed to players and KRA |
| `kyc_reviewer` | see submitted identity details (every view is audited as unmasked), approve or reject them with a reason |
| `payments_operator` | run the payments and game reconciliations, record reversals, resolve unconfirmed top-ups, credit or refund suspense payments, re-check paybill payments |
| `draw_official` | sign frozen draws, approve conducted draws, void a draw before its round (with a reason) |
| `payout_operator` | approve or reject held payouts, settle manual payouts from the statement, retry failed prizes |
| `tax_officer` | prepare KRA batches, download batch files, record remittances |
| `alerts_operator` | acknowledge alerts (lifts a payout freeze caused by a failed reconciliation) |
| `support` | close accounts whose holders asked |
| `auditor` | read every list (Finance included) and run reconciliations; changes nothing |

**Two-person rules hold in the back office as in `lotteryctl`:** two different officials sign, and two approve, each
draw; the person who records a KRA remittance must not be the one who prepared the batch. Give `draw_official` and
`tax_officer` to at least two people each, and keep two admins so one can always restore the other.

Staff accounts should have 2-step verification on (the platform cannot require it of Google or Apple). Apple's
"Hide My Email" gives a relay address: invite that address, or ask the member to share their real email with Apple.

## Setting up Google and Apple (once, by the owner)

**Google** (Google Cloud console → APIs & Services → Credentials → Create OAuth client ID → Web application):
authorised JavaScript origins = the back office's address (`https://lottery-backoffice….onrender.com`, later the
Cloudflare domain) and `http://localhost:5173` for local testing; no redirect URI is needed (popup). The consent
screen needs the app name and a support email. The client id goes into `STAFF_GOOGLE_CLIENT_ID` (api) and
`VITE_GOOGLE_CLIENT_ID` (back office).

**Apple** (Apple Developer → Certificates, Identifiers & Profiles → Identifiers): an App ID with Sign in with Apple,
then a **Services ID** (e.g. `ke.bahati.backoffice`) with Sign in with Apple configured: domain = the back office's
host, return URL = its address with a trailing `/`. Apple needs https, so it cannot be tried on localhost. The
Services ID goes into `STAFF_APPLE_CLIENT_ID` (api) and `VITE_APPLE_CLIENT_ID` (back office).

## Configuration

API (`render.yaml`, `lottery-api`):

```
STAFF_GOOGLE_CLIENT_ID=<google web client id>         # either or both
STAFF_APPLE_CLIENT_ID=<apple services id>
STAFF_BOOTSTRAP_ADMIN=itsivali@gmail.com              # made an admin while there is none
STAFF_DATABASE_URL=postgres://<worker login>@…        # the worker's role; omit only in single-role mode
CORS_ALLOWED_ORIGINS=https://<back-office origin>,https://<player web origin>
```

Back office (`render.yaml`, `lottery-backoffice` static site): `VITE_API_BASE_URL`, `VITE_GOOGLE_CLIENT_ID`,
`VITE_APPLE_CLIENT_ID` (build-time). Never set `VITE_DEV_OIDC_*` or `STAFF_DEV_OIDC_*` outside local development.

## Local

`scripts/dev-backend.sh` starts the stack and seeds the staff (`scripts/seed-staff.sh`); `scripts/dev-frontend.sh
--backoffice` serves http://localhost:5173. Locally the compose Keycloak realm stands in for Google and Apple
("Sign in with the local staff realm"): `ada` is the admin, `alice` (draws, auditor, finance), `bob`, `kim` (identity
review), `paula`, `otis`, `tara`, `tom`, `sam`, all with password `local-dev-staff`. `scripts/seed-staff.sh token
<user>` prints a session token for curl. To try the real Google button locally, set `STAFF_GOOGLE_CLIENT_ID` before
starting the stack and `VITE_GOOGLE_CLIENT_ID` in `apps/backoffice/.env.local`.

## Daily operations

- **Finance:** the *Finance* page reads every figure from the ledger for the last 7, 30 or 90 Nairobi days, with a
  daily chart and table, and shows what is owed now (wallets, excise and withheld tax, prize pools, suspense).
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
