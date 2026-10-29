# Runbook: the back office

The back office (`apps/backoffice`) is how staff run the operations that `lotteryctl` performs (ADR 0013, 0018). Every
action is checked against the staff member's role by the API and audited as `staff:<id>`. `lotteryctl` still works for
emergencies (`-operator <your staff id>`, audited as `staff:<id>` like the back office).

## Sign-in and roles

Staff sign in with their **email address and password** (ADR 0019); **Google or Apple** are offered as well when the
owner has set them up (ADR 0018). Nobody can sign in uninvited: an admin adds each person on the *Staff* page by
email address, gives them one of the two roles, and may set a **temporary password** which they tell the person out
of band (never in a chat that keeps history). The person must change it at their first sign-in before they can do
anything else. An admin can set a new temporary password at any time (*Reset password*), which also signs the person
out everywhere. A person without a password signs in with the Google or Apple account of that address.

The back office shows each member by **name**, never by email address: an invite needs a name, a member without one
(the bootstrap admin, say) is asked for it right after their first sign-in, anyone can change their own from the
account menu (*Change name*), and an admin can rename a member on the *Staff* page. Every change is audited
(`staff.name`).

**Locked out (the only admin has lost their password):** nobody can reset it from the back office, and
`lotteryctl staff password` needs database access. Without that, set `STAFF_BOOTSTRAP_RESET=true` on the api (Render →
lottery-api → Environment), with `STAFF_BOOTSTRAP_PASSWORD` set to a fresh temporary password (12 characters or more),
and deploy. On start the api gives the bootstrap admin that temporary password again, re-enables them and ends their
sessions (audited as the system). Sign in with it, choose a password, then **remove `STAFF_BOOTSTRAP_RESET`** and
deploy again: while it is on, every start resets the password and logs a warning. Note that `STAFF_BOOTSTRAP_PASSWORD`
on its own only applies while the admin has no password at all: changing it after the first sign-in does nothing.

| Role | Can |
|---|---|
| `staff` | every operational queue: identity review (each view audited as unmasked; approve or reject with a reason), payments (reconciliations, reversals, unconfirmed top-ups, suspense, paybill re-checks), draws (sign, approve, void with a reason), payouts (approve, reject, settle from the statement, retry prizes), KRA tax (prepare batches, download files, record remittances), alerts (acknowledge), account closures; and the *Finance* dashboard |
| `admin` | everything staff can, and the *Staff* page (invite, roles, disable and re-enable, reset passwords; never their own) and the *App metrics* page |

**Two-person rules hold in the back office as in `lotteryctl`:** two different people sign, and two approve, each
draw; the person who records a KRA remittance must not be the one who prepared the batch. Keep at least two staff
members for the draws and the tax batches, and two admins so one can always restore the other.

Passwords: 12 characters or more (a sentence is fine), nothing common, never your email address. Google or Apple
accounts should have 2-step verification on; the platform cannot require a second factor for password sign-in, so
prefer Google or Apple where they are set up. A lost password is reset by an admin; a lost admin password by another
admin, or, for the bootstrap admin with no password yet, by `STAFF_BOOTSTRAP_PASSWORD` (below).

**Every administrator locked out** (passwords lost, Google and Apple not configured): from a shell with the api's
environment (`DATABASE_URL` and `AUTH_KEY` at least; on Render, the api service's shell):

```
lotteryctl staff password -email you@example.com -admin
Temporary password (12 characters or more): ‹typed, not echoed into history›
```

It gives that member a temporary password, re-enables them, ends their sessions and, with `-admin`, makes them an
administrator (creating the member if none exists). The next sign-in must change the password. The action is audited
as `staff.emergency_password` by `lotteryctl`. `-password P` is accepted for scripts but lands in the shell history.

## Setting up Google and Apple (once, by the owner)

**Google** (Google Cloud console → APIs & Services → Credentials → Create OAuth client ID → Web application):
authorised JavaScript origins = the back office's address (`https://backoffice.tajirika.co.ke`) and
`http://localhost:5173` for local testing; no redirect URI is needed (popup). The consent
screen needs the app name and a support email. The client id goes into `STAFF_GOOGLE_CLIENT_ID` (api) and
`VITE_GOOGLE_CLIENT_ID` (back office).

**Apple** (Apple Developer → Certificates, Identifiers & Profiles → Identifiers): an App ID with Sign in with Apple,
then a **Services ID** (e.g. `ke.tajirika.backoffice`) with Sign in with Apple configured: domain = the back office's
host, return URL = its address with a trailing `/`. Apple needs https, so it cannot be tried on localhost. The
Services ID goes into `STAFF_APPLE_CLIENT_ID` (api) and `VITE_APPLE_CLIENT_ID` (back office).

## Configuration

API (`render.yaml`, `lottery-api`):

```
STAFF_BOOTSTRAP_ADMIN=itsivali@gmail.com              # made an admin while there is none
STAFF_BOOTSTRAP_PASSWORD=<a long temporary password>   # their temporary password while they have none; change it at the first sign-in
STAFF_GOOGLE_CLIENT_ID=<google web client id>         # optional, either or both
STAFF_APPLE_CLIENT_ID=<apple services id>
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
