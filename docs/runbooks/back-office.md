# Runbook: the back office

The back office (`apps/backoffice`) is how staff run the operations that `lotteryctl` performs (ADR 0013, 0018). Every
action is checked against the staff member's role by the API and audited as `staff:<id>`. `lotteryctl` still works for
emergencies (`-operator <your back-office email>`: it asks for your back-office password, signs you in and audits the
action as `staff:<id>` like the back office; ADR 0032).

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

**Forgotten password:** *Forgotten it?* on the sign-in page emails the member a link (from `SMTP_FROM`, to the
address they sign in with) that opens `/reset-password` and works for 30 minutes; the new password follows the staff
rules and ends their other sessions. The answer is the same whether or not the address is a member's. It needs the
api's `SMTP_*` and `BACKOFFICE_URL` set; without them an admin sets a temporary password instead.

**Players who forgot their password** have no email, so staff reset it from the *Players* page (*Temporary password*):
check who is asking against the name and ID number shown, give them a temporary password by phone (never SMS or
chat) with a reason for the audit log. Every session of theirs ends and the app makes them choose their own password
at the next sign-in before anything else.

**Locked out (the only admin has lost their password):** nobody can reset it from the back office, and
`lotteryctl staff password` needs database access. Without that, set `STAFF_BOOTSTRAP_RESET=true` on the api (Render →
lottery-api → Environment), with `STAFF_BOOTSTRAP_PASSWORD` set to a fresh temporary password (12 characters or more),
and deploy. On start the api gives the bootstrap admin that temporary password again, re-enables them and ends their
sessions (audited as the system). Sign in with it, choose a password, then **remove `STAFF_BOOTSTRAP_RESET`** and
deploy again: while it is on, every start resets the password and logs a warning. Note that `STAFF_BOOTSTRAP_PASSWORD`
on its own only applies while the admin has no password at all: changing it after the first sign-in does nothing.

| Role | Can |
|---|---|
| `staff` | every operational queue: identity review (each view audited as unmasked; approve or reject with a reason), payments (reconciliations, reversals, unconfirmed top-ups and players' receipts, the M-Pesa statement import, suspense, paybill re-checks), draws (sign, approve, void with a reason), payouts (approve, reject, settle from the statement, retry prizes), KRA tax (prepare batches, download files, record remittances), alerts (acknowledge), account closures; and the *Finance* dashboard |
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

## Players who cannot sign in

A player who forgot their password resets it themselves with the "Forgotten your password?" link in the app, which
emails them (ADR 0021). When that fails them (no email address on the account, or no access to it), the Players page
→ *Temporary password* gives them one to sign in with once; check the caller against the name and ID shown first and
tell them the password by phone. The page shows and searches by email address.

## Setting up Google and Apple (once, by the owner)

These are the back office's own client ids. The player apps' Google and Apple sign-in is a separate set, in
`docs/runbooks/player-sign-in.md`.

**Google** (Google Cloud console → APIs & Services → Credentials → Create OAuth client ID → Web application):
authorised JavaScript origins = the back office's address (`https://backoffice.tajirika.co.ke`) and
`http://localhost:5173` for local testing; no redirect URI is needed (popup). The consent
screen needs the app name and a support email. The client id goes into `STAFF_GOOGLE_CLIENT_ID` (api) and
`VITE_GOOGLE_CLIENT_ID` (back office).

**Apple** (Apple Developer → Certificates, Identifiers & Profiles → Identifiers): an App ID with Sign in with Apple,
then a **Services ID** (e.g. `ke.co.fourdraws.backoffice`) with Sign in with Apple configured: domain = the back office's
host, return URL = its address with a trailing `/`. Apple needs https, so it cannot be tried on localhost. The
Services ID goes into `STAFF_APPLE_CLIENT_ID` (api) and `VITE_APPLE_CLIENT_ID` (back office).

## Configuration

API (`render.yaml`, `lottery-api`):

```
STAFF_BOOTSTRAP_ADMIN=admin@tajirika.co.ke           # made an admin while there is none
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

- **Finance** (owner, 2026-10-31, ADR 0023): the *Finance* page reads every figure from the ledger for a range of
  Nairobi days (the last 7, 30 or 90 days, this or last month, this quarter, the year to date, or any two dates up to
  366 days apart), cut by day, Monday-start week or calendar month. *Overview* has the money in and out, GGR per
  period and what is owed now; *Top-ups* the M-Pesa funnel (started, credited, failed, cancelled, expired,
  unconfirmed, reversed, still pending, and the median time to credit); *Payouts & tax* the payout funnel, each prize
  pool and the withholding tax position (batches awaiting remittance, withholding not yet batched, the oldest
  unremitted item's age); *Trial balance* every ledger account as at the range's end, with a warning when debits and
  credits differ (they never should; it means a posting is broken, so stop and raise it); *Reconciliations* every
  stored run of each area, newest first, with *Run now*. **For an audit**, use the two export buttons: *Export
  figures* is the page as CSV (periods, totals, trial balance), *Export ledger* every ledger entry of the range (at
  most 100,000; narrow the range if refused). Both answer with the file's SHA-256 (`X-File-SHA256`), and each export
  is recorded in the activity log (`finance.exported`, `ledger.exported`) with who, the range, the row count and the
  hash, so a file handed to an auditor can be matched to its download.
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
- **Plays** (owner, 2026-10-30): every play, newest first, with the player's name (from their identity submission),
  number, time, price and where the wheel stopped; search by number or player id, or pick a play date. Viewing it is
  audited, because it unmasks names.
- **Top-ups:** every M-Pesa prompt and paybill payment with its outcome in words (M-Pesa's result code, the failure
  reason) and receipt. *Sandbox* marks a top-up credited by sandbox settlement: no money moved.
- **System status:** the settings the api runs with (sandbox or production M-Pesa, sandbox settlement, the top-up
  cooldown, price of a play, which sign-ins and integrations are on; never a secret), failing background jobs with
  their last error, the last run of each reconciliation, and counts that link to their pages.
- **Alerts on every page:** the sidebar counts open alerts; a paging alert, or one that froze payouts, is shown at the
  top of every page until someone acknowledges it on *Alerts*. Polled every minute.
- **Activity log** (administrators): the audit trail, newest first, searchable by action, id or actor and filtered by
  who acted, by day, by action family and by entity (below). It cannot be changed or deleted; it can be downloaded.
- **Session timeouts:** a session that lapses (with the tab open, or while it was closed) or that the server ends
  sends the member to sign in with *Your session timed out*; the player app says the same when its session ends.

The draw entry list (which ties entries to players) is not in the back office: it is for the regulator
(`lotteryctl draws entries`).

## Running an audit (owner, 2026-10-31; ADR 0026)

The back office answers "what happened to this player, this money, or on this day, and who did it", and lets you
take the evidence away as files. Every file carries its SHA-256 (`X-File-SHA256`, shown after the download) and
its row count, and the download itself is recorded in the activity log, so a file can later be tied to the log it
came from.

**A player.** *Players* → search by phone number, email or user id → *Open* (or follow a `/players?player=<id>` link
from the activity log). The sheet shows the account (name and ID number from the identity submission, else sign-up),
the balance and anything owed after a reversed top-up, and tabs for the wallet's last 50 movements, the last 20
top-ups (each opens on *Top-ups*), plays, payouts, the identity history (submissions' outcomes, the last 10
sign-ins with the device's verdict, the last 20 account status changes with who made them and why (ADR 0030), a
closure request), the deposit limits and any time-out or self-exclusion, and
the last 50 audit records about the player. Suspend, reinstate and temporary password work from the sheet too.
*Download player pack (CSV)* gives every row of every list in one file for an auditor or a data-subject request:
columns `section` (player, wallet_transaction, top_up, play, payout, kyc_check, session, status_change, deposit_limit, restriction,
event), `id`, `at_eat`, `kind`, `status`, `amount_kes`, `reference`, `details_json`. Opening the sheet is audited
(`identity.player_viewed`, and `kyc.unmask` for the name); the pack as `identity.player_exported`.

**Money, or a day.** *Activity log* (administrators): set *From* and *To* (Nairobi days, inclusive), pick an action
family (`payment.` for every top-up event, `payout.`, `draw.`, `kra.`, `staff.`, …), or name the entity (type
`payment` and the payment's id; type `user` and the player's id) or the actor (a player's id, or `staff:<id>` -
staff show by name). The search box still matches fragments. Ids in the rows link to the top-up, player, payout,
draw or alert they name. *Download CSV* exports what the filters show, oldest first, as `activity-<from>-<to>.csv`
(columns id, at_eat, actor_type, actor_id, actor_name, action, entity_type, entity_id, details_json), up to 100,000
records; narrow the filters if it is refused with "too many records". The export is recorded as `activity.exported`
with the filters, the row count and the hash.

**Figures.** *Finance* exports the summary and the full ledger for a range (ADR 0023); the KRA batch files are on
*KRA tax*. Together with a player pack and the day's activity file, that is what an auditor needs to trace a
shilling from the M-Pesa receipt to the wallet, the play, the prize and the payout.
