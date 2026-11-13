# Status

Read this first at the start of every session.

- **Owner decisions 2026-10-24 (ADR 0011):** paybill **C2B** (menu payments) is shelved until a paybill exists. Top-ups
  are **STK prompts into the paybill** (174379 in the sandbox). Buy Goods (`DARAJA_TILL_NUMBER`) is built but off: the
  sandbox refuses it (`Invalid TransactionType`), so it waits for a real till.
- **Phase:** 1 — Foundations: complete. **Phase 2 — Payments: complete (2026-10-24).** **Phase 3 — Game and tickets: complete
  (2026-10-24).** **Phase 4 — Draws: built** (`docs/checkpoints/phase-4.md`): slices 1 (drand), 2 (lifecycle) and 3
  (public results API, `tools/verifier draw`, daily draw reconciliation, late-step monitoring; branch
  `feat/draws-results`, 2026-10-25). Not yet seen live: a draw signed and approved by two officials on Render.
  **Phase 5 — Payouts: built (2026-10-25)** (`docs/checkpoints/phase-5.md`, runbook `docs/runbooks/payouts.md`): B2C
  client and mock, withdrawals, prize payouts, KRA withholding feed, payout reconciliation — merged (#34–#37,
  2026-10-25). Follow-ups on the owner's decisions of 2026-10-25: prizes repay winners' debt and certificates name no
  winners (PR #38); KRA remittance batches (`docs/runbooks/tax.md`) and alerts with a payout freeze
  (`docs/runbooks/alerts.md`) (PR stacked on #38). To try payouts in the sandbox: set `DARAJA_B2C_SHORTCODE=600992`
  on Render. **Phase 6 — player web app and back office: in progress (started 2026-10-26 on the owner's instruction, with phases 7 and 8 to follow)** (`docs/checkpoints/phase-6.md`, ADR 0012): slice 1 (player API: wallet, account, top-up list, closure request, CORS) on `feat/player-account-api`; slice 2 (the Flutter player app for iOS, Android and web, design system in `design-system/lottery-platform/`), slice 3 (back-office API with Keycloak staff sign-in and roles, ADR 0013) and slice 4 (`apps/backoffice`) on `feat/mobile-app`. **Phase 7 — device trust: built** (`docs/checkpoints/phase-7.md`, ADR 0014) on `feat/device-attestation`: attestation challenges, App Attest and Play Integrity verifiers, `ATTESTATION_MODE`; native attestation, SSO client ids and push wait on the owner's Apple/Google/Firebase accounts. `make e2e` covers play, withdrawal, attestation and the back office.
  Phase 2 detail (`docs/checkpoints/phase-2.md`). Slices 1
  (Daraja client + mock), 2 (STK top-up, ADR 0006) and 4 (reversals, player debt, internal reconciliation, ADR 0007)
  are merged and live. **Slice 3 (C2B paybill, ADR 0008) is merged and live (2026-10-24, #18, #19)**; its sandbox
  end-to-end run is blocked on Daraja: the sandbox accepts registrations, simulations and status requests but has not
  called back yet (below). **Slice 5 (realtime: `GET /v1/payments/events`, server-sent events fed by Postgres
  notifications, ADR 0010) is built** (branch `feat/payments-realtime`).
- **Repo state:** the GitHub repository was **deleted and recreated on 2026-10-22** (to drop co-author attribution that
  GitHub kept in old PR refs and caches) and `main` was pushed from the local clone. PR numbers below #8 in these docs
  refer to the deleted repository. Commits and PRs carry only the owner's name.
- **Deployed (verified 2026-10-24):** Blueprint `lottery-plarform` (database, Key Value); `lottery-api` (recreated 2026-10-24, `srv-dbe9gavb9u1s738ttucg`) (`https://lottery-api-4u9g.onrender.com`,
  free, Oregon) builds from `main` (repository reconnected by the owner on 2026-10-24); migrations at version 17;
  sandbox top-ups and paybill on (`payments=true paybill=true`, shortcode 600992); client address from
  `CF-Connecting-IP`. Free Postgres `lottery-db` (PG 18) and Key Value `lottery-cache`.
- **Renamed to 4draws (2026-11-11, owner):** the product was Tajirika until now. The name, the logo (a 4 in a ring of
  four segments, one gold, on indigo: `design-system/lottery-platform/logo`), every app icon, the M-Pesa prompt's
  account reference and description, the mail subjects and the app ids (`ke.co.fourdraws.app` on Android and iOS,
  `ke.co.fourdraws.web` / `.backoffice` for Apple sign-in; a package segment cannot start with a digit) are 4draws.
  The domain `4draws.co.ke` was registered the same day and has no DNS yet: the sites stay on `tajirika.co.ke` until
  the cutover (its own change: Render custom domains, DNS at Safaricom, `render.yaml` URLs, CORS, canonical and
  sitemap addresses, the Daraja callback and paybill URLs registered again). The public fairness repository keeps its
  name (`itsivali/tajirika-fairness`) until the owner renames it.
- **Player app, Material 3 (2026-11-11, owner):** after two same-day redesign passes (soft glows, then a full 3D
  pass that the owner found too much), the app is a native-feeling Material 3 app in the logo's indigo: flat filled,
  outlined and tonal buttons, a standard navigation bar, each platform's own page transition, tonal icon discs, the
  balance on an indigo hero card, the wheel squarely centred in its dial. The only 3D left is the gold coin beside
  the balance. Android has a launch screen in the page colour. `design-system/lottery-platform/MASTER.md` v4. The
  back office still wears coral and violet until its own pass.
- **One design language, app and back office (2026-11-11, owner):** the back office wears the app's indigo
  (`apps/backoffice/src/index.css`), and both follow MASTER.md v4.1: signed-out pages are one white card in the
  middle of the indigo gradient under a deep shadow (`AuthPage` in the app, `AuthShell` in the back office); cards
  stand off the page on a contact shadow plus a wide soft one, and tappable cards come forward under the pointer;
  pages, tabs, dialogs and loading states fade and settle instead of cutting. The back office's 3D tilt and swing are
  gone; its gold coin stays.
- **Policy pages complete (2026-11-11, owner):** `/privacy/`, `/terms/`, `/responsible-gambling/` and
  `/delete-account/` are full documents in the operator's name, **4 DRAWS TICKET LTD**, version 1.0, with no open
  items; each still states that the service is not yet licensed. They were written for this platform, structured as
  Kenyan banks' and telcos' documents are, the deletion page after Meta's. **Not reviewed by counsel.** Things they
  state that the owner must make true or correct before a licence: the mailboxes `support@4draws.co.ke` and
  `privacy@4draws.co.ke` (they do not exist; the domain has no working DNS yet); closure "normally within 30 days" and
  backups overwritten within 90 (commitments nobody has measured); retention of seven years (POCAMLA) and five (tax),
  which ADR 0015 leaves to counsel; deleting the kept records when those periods end (not built: ADR 0037 erases
  at closure what the law does not require, since 2026-11-11); staff and their families barred from playing; the helplines named on the
  responsible gambling page. The company's registration number, postal address, licence number and ODPC registration
  are not on the pages because they are not known here.
- **Erasure at closure (2026-11-11, ADR 0037, migration 00056):** closing an account now erases, in the same
  transaction, its password, sessions, devices, Google/Apple links, photo, email and reset links, through one database
  function that refuses any account that is not closed; the money's records, identity checks and the audit log stay.
  Accounts closed earlier were erased by the migration. Not built: purging the kept records when retention ends.
- **A copy of your data by email (2026-11-11, ADR 0038, migration 00057):** Account → Get a copy of your data
  (`POST /v1/account/data-export`) emails the player a readable report (an HTML page with a table per kind of record)
  and the same data as JSON, to the account's own address, once a day; the ID number as its last three digits. Recorded in `identity.data_exports` and the audit log.
  **Does nothing in production until SMTP is configured** (the route answers 503 and names the privacy address).
- **Moved to 4draws.co.ke (2026-11-12):** Safaricom repaired the domain's DNS zone (their cPanel refused every record
  with "the serial argument cannot be empty"; a support ticket fixed it). The records point at Render (`@` A
  216.24.57.1; `www`, `api`, `backoffice` CNAMEs to the services' onrender.com names), Render issued certificates,
  and the repository's addresses moved: `PUBLIC_BASE_URL` (M-Pesa callbacks), the apps' API address, links in mail,
  the canonical address, sitemap, robots.txt and policy pages. `tajirika.co.ke` keeps serving; the API allows both
  origins. Still to do, in order: register the paybill (C2B) addresses again on the new host; Search Console change
  of address; verify `4draws.co.ke` with the mail provider and move `SMTP_FROM` (it is `4draws
  <noreply@tajirika.co.ke>`); sign-in providers' origins. `docs/runbooks/domain-cutover.md`.
- **Google sign-in for players (2026-11-12):** on, Apple off. `GOOGLE_CLIENT_ID` on `lottery-api` is the OAuth web
  client "4draws web" (origins `https://4draws.co.ke`, `https://tajirika.co.ke`); an Android client "4draws Android
  (upload key)" exists for `ke.co.fourdraws.app` with the upload key's SHA-1. Still the owner's: the Google project's
  consent screen is in **Testing** (only listed test users can sign in) and is named "Lottery back office"; publish
  it after completing Branding. Once the app is on Play, add a second Android client with Play App Signing's SHA-1
  (Play Console → App integrity), or Google sign-in fails in the store build. No `APPLE_*` setting is set.
- **Google sign-in finds the account by its email (2026-11-12, ADR 0039):** a Google identity not yet linked signs
  in the account that has its Google-verified address (gmail.com or a Workspace domain) and is linked to it; only
  with no such account is a new one offered.
- **Email addresses are confirmed by an emailed link (2026-11-13, ADR 0040):** sent at sign-up, on a change of
  address and on request (24 hours, once; `/verify-email`). Until confirmed, an address still gets password-reset
  links, but Google sign-in does not find the account by it and a copy of the player's data is not sent to it.
  Addresses given before this are unconfirmed; the app says so under Account. Player emails and the staff password
  reset now use one branded layout with the logo inside the message (`internal/platform/mail/notice.go`).
- **An unconfirmed address is only a claim (2026-11-13, ADR 0041):** several accounts may give one address; the one
  that confirms it keeps it and the others lose it. Only a confirmed address refuses another sign-up. The back
  office marks players whose address is not confirmed.
- **Play Console (2026-11-11):** the owner's developer account is a personal one ("willis ivali"). The app is to be
  created as 4draws, `ke.co.fourdraws.app`, English (UK), App, Free; the three declarations are the owner's to
  accept. A release build signed with the upload key builds locally. Not yet possible: production release (needs
  the licence, Google's gambling application and, for a personal account, a 12-tester closed test of 14 days).
- **Domain and plans (2026-10-29):** the product (then Tajirika) lives at `tajirika.co.ke` (registered with
  Safaricom, DNS there): https://tajirika.co.ke (web app; www redirects), https://api.tajirika.co.ke and
  https://backoffice.tajirika.co.ke, with Render certificates; the onrender.com addresses keep working. `lottery-api`
  runs on the paid `0.5c-512mb` plan and `lottery-db` on `0.1c-256mb` (recreated empty on 2026-10-28); the free-database
  rotation is off.
- **Keys:** `DATA_KEY` and `IDENTITY_KEY` were handed to the owner for the password manager; save `AUTH_KEY` too.

- **Compliance and release readiness (started 2026-11-07, licensing pending):** audit baseline and backlog in
  `docs/release/implementation-roadmap.md`; requirements from LN 112/2026 and others in
  `docs/compliance/requirements-matrix.md`; evidence in `docs/compliance/evidence-register.md`. Owner decisions:
  operating mode `disabled | sandbox | live` (sandbox only with sandbox Daraja; live needs licence evidence and two
  approvals); 18+ declaration with date of birth at sign-up, verified ID before money in when live; one PR per slice.
  Critical for live: LN 112 reg 42 (player data in Kenya; production is in Oregon), reg 82 (ID, age, location before
  deposit), reg 27 (charity share for public lotteries).
- **Hosting (owner, 2026-11-07):** production moves from Render (Oregon) to **hosting in Kenya** for LN 112 reg 42
  (player data in Kenya). Safaricom's cPanel hosting packages cannot run the platform (long-running Go api and workers,
  PostgreSQL with roles and triggers, Valkey, containers): it needs virtual machines with root access in a Kenyan data
  centre. Next: ask Safaricom Business for cloud virtual machines (Nairobi/Kisumu); fallback Angani or Lineserve
  (Nairobi), location confirmed in writing. Plan: slice S12 in `docs/release/implementation-roadmap.md`.
- **Operating mode (ADR 0027, 2026-11-07):** `OPERATING_MODE` (default `disabled`) caps what the api may do; staff can
  stop money moving from System status or `lotteryctl compliance stop`; an admin resumes; live needs production
  Daraja, `REGULATORY_LICENCE_REFERENCE` and two admins (`docs/runbooks/operating-mode.md`). **Render must have
  `OPERATING_MODE=sandbox`** (render.yaml sets it; check the dashboard after the Blueprint syncs), or the demo stops
  taking top-ups and plays. `GET /v1/status` shows the mode.
- **M-Pesa callbacks are claims (ADR 0028, 2026-11-07):**
  - The first callback is kept, and a different later one is audited and ignored.
  - A callback naming another amount than the prompt, or no valid receipt, is not credited (`unconfirmed`,
    `amount_mismatch`/`receipt_invalid`).
  - A late or orphan callback is confirmed with Transaction Status before it credits. In the sandbox
    (`DARAJA_SANDBOX_SETTLE`) it still credits at once.
  - The database enforces the top-up state machine (migration 00046).
  - The hook secret is redacted from traces and logs.
- **Age at sign-up (ADR 0029, 2026-11-07):**
  - Sign-up requires `date_of_birth` and `adult_declaration: true`; under 18 is refused by the server. The date is
    stored encrypted in `kyc.age_declarations` (migration 00047).
  - Identity verification needs a date of birth, taken from sign-up when it is left out.
  - In `live`, top-ups, paybill payments and plays need a verified ID.
  - App builds from before this cannot sign up new accounts until updated.
- **Eligibility checked again when money moves (ADR 0030, 2026-11-08):**
  - The payout send job and `Approve` check the account again. One that cannot be paid now (suspended, closed,
    identity not verified, number changed) is held, not sent (migration 00048 allows `pending → held`).
  - The top-up prompt job re-checks the operating mode, the account, time-outs and self-exclusion; a refused top-up
    fails without a prompt.
  - Every account status change is kept in `identity.user_status_history` (migration 00049), written by a trigger,
    and shown on the back office player timeline and in the player pack.
  - Self-exclusion ends every session at once; the app says why on the sign-in screen.
- **Refund to source and the country signal (ADR 0031, 2026-11-08):**
  - An admin can send a balance the account cannot withdraw back to the number that paid it in; it is held until
    someone else approves it (migration 00050).
  - Sign-ups, top-ups, plays and withdrawals record the country Cloudflare reports (`GEO_COUNTRY_HEADER=CF-IPCountry`
    on Render). In `live`, everything but KE is refused, except withdrawals; `live` will not start without the header.

- **Two people for payouts (ADR 0032, 2026-11-09):**
  - A held payout is sent after two different staff approvals; a manual payout is settled when a second person
    confirms the first one's decision (migration 00051).
  - `lotteryctl -operator` is the back-office email address; it asks for the password and records the action under
    the signed-in staff id.
  - Staff can record a self-exclusion a player asks for; nothing lifts one.
- **A payout's receipt is proved by M-Pesa (ADR 0034, 2026-11-09):** a manual payout is settled as paid only when
  Transaction Status confirms the receipt staff found on the statement (amount and number included); migration 00052.
- **Transaction monitoring (ADR 0033, 2026-11-09):** five rules run hourly and open cases on the back office's
  Compliance page; a case can hold a player's money (no top-ups, paybill, withdrawals or payouts), lifted by someone
  else. Thresholds are placeholders pending the compliance owner (`docs/runbooks/compliance-monitoring.md`); migration
  00053.

- **Google photos and first passwords (ADR 0035, 2026-11-11):** the Google account's photo shows in the app and the
  back office (Google's image host only; migration 00054); a Google or Apple account can set a password within 10
  minutes of signing in.

- **Back office controls (ADR 0036, 2026-11-11):** money switches on the System page pause top-ups, plays, withdrawals
  or payouts on their own (anyone on staff pauses, an admin resumes; audited and alerted; migration 00055); an admin
  edits the monitoring rules, turns them on or off and records the compliance owner's approval; failing background jobs
  can be retried or cancelled by an admin.

- **Android release readiness (S6, 2026-11-11):** release builds are signed with the upload key or stop (never the
  debug key); target and compile SDK 36 pinned; no backup or device transfer; the version code is the CI run number.
  Tag builds need the owner's upload key in GitHub secrets (`docs/mobile/android-release.md`).

- **iOS release readiness (S7, 2026-11-11):** privacy manifest bundled (draft for counsel), export flag, iPhone-only
  portrait, bundle name 4draws, Google URL scheme; the Apple team id and Google iOS client come from an uncommitted
  `Local.xcconfig` (`docs/mobile/ios-release.md`). Archiving and TestFlight wait for the owner's Apple account.

- **Policy pages and in-app links (S8, 2026-11-11):** `tajirika.co.ke/privacy/`, `/terms/`, `/responsible-gambling/`
  and `/delete-account/` are plain pages in the web build, marked DRAFT with `[TO CONFIRM]` items for counsel; the app's
  Account → About links to them, the closure screen says what is kept, and Home shows a demo banner in sandbox (and a
  paused one when stopped). Data inventory: `docs/security/privacy-data-inventory.md`.

## Before the API is exposed to players

1. **Trusted proxy: fixed.** Measured on the live service (`docs/runbooks/trusted-proxy.md`): the rightmost
   `X-Forwarded-For` entry was a rotating Render balancer address; the api now reads `CF-Connecting-IP`, which
   Cloudflare refuses to accept from clients. **Probe 1 passed** (60 × 401 then 429 with forged `X-Forwarded-For`).
   Remaining: **probe 2** from a second network (5 failed logins from a phone on mobile data must all be 401, not 429).
2. **Free Postgres:** rotated on 2026-10-22 (first supervised run, data intact); now expires **2026-11-21** and the next
   rotation runs by itself around 2026-11-18. **Automated** by `db-rotate` (`docs/runbooks/free-database-rotation.md`): daily
   check, rotation (backup, verify, recreate, restore, switch) when 3 days or fewer remain. Secrets are set; the
   passphrase must stay in the owner's password manager. Upgrading the database to a paid plan ends the need for it.
3. **Database roles:** Render's user has `CREATEROLE`; the original instance had the four app roles. Since the rotation
   the instance runs in **single-role mode** (no `app_*` roles; every trigger and constraint holds), as documented.

## Decisions taken (see ADR 0005)

- Accounts are created with **phone number + password** (argon2id, peppered). **No SMS, no OTP, no TOTP for now.**
- Phone ownership is proved by the **M-Pesa STK Push confirmation at the first top-up** (Phase 2 calls
  `identity.Service.MarkPhoneVerified`). The proof is the push target (the phone that approved the prompt); the
  callback's payer number is only cross-checked when present, because Safaricom may mask it.
- **KYC is reviewed by hand (owner, 2026-10-24, ADR 0009):** no registry provider; submissions wait in `manual_review`
  for `lotteryctl kyc approve|reject` (back office later). Google/Apple sign-in ids come with the web app; SSO is off.
- Free-tier deployment: migrate on start (advisory lock), workers in-process, free Postgres + Key Value.
- **Draws (owner, 2026-10-26, ADR 0017):** weekly, monthly and quarterly periods are each player's own, 7, 30 and 90
  days back to back from their first play; a draw closes each day for the players whose period ends that day.
- **Plays (owner, 2026-10-29):** a spin costs **KES 1** from 2026-10-29 (migration 00036; it was KES 100 from
  2026-10-28, migration 00033), so a top-up of a shilling (`TOP_UP_MIN_KES=1`) is enough to play; up to four a day, and
  paid from the player's own M-Pesa number: when the wallet cannot cover the price the
  app charges exactly the price as a top-up and spins the moment it is credited ("Pay KES 1 with M-Pesa and spin");
  a wallet that can cover it still pays from the wallet.
- **Look (owner, 2026-10-28):** colourful, premium and light only, on a white background, in the player apps and the
  back office: the "Sunset" palette (coral primary, violet accents, gold decorative; `design-system/lottery-platform/
  MASTER.md` v2.1). Dark mode is gone. The web app links to the back office (`BACKOFFICE_URL` dart-define; Render
  `lottery-web` points at `https://backoffice.tajirika.co.ke`).
- **Staff (owner, 2026-10-28, ADR 0019; was ADR 0018):** two roles, `admin` (everything, staff management, app
  metrics) and `staff` (every operational queue and the finance dashboard). Staff sign in with email and password
  (temporary ones set by admins, changed at the first sign-in); Google and Apple are optional. admin@tajirika.co.ke ("Administrator"; was itsivali@gmail.com until 2026-10-29) is
  the bootstrap admin; `STAFF_BOOTSTRAP_PASSWORD` (Render secret, to set) is their first temporary password. The back
  office has Finance and App metrics dashboards and deploys as the `lottery-backoffice` static site.
- **Names (owner, 2026-10-30):** the account screen shows the player's name above the number; the name is the one on
  their identity submission (its first two words, `GET /v1/account` `first_name`/`second_name`), so it appears once
  they submit their ID. Nothing is asked at sign-up (the sign-up fields of 2026-10-29 were removed the next day;
  migration 00039 drops the columns of 00038).
- **Identity verified before the first withdrawal (owner, 2026-10-29, ADR 0022):** top-ups and plays no longer wait for
  the manual ID review; only withdrawals and payouts do. The review queue therefore holds players who want to
  withdraw. Automating the check means a registry vendor (IPRS through Smile ID or similar, per-check fee) behind
  the kept `KYC_API_URL` gateway: the owner's decision.
- **Email at sign-up and self-service password reset (owner, 2026-10-29, ADR 0021):** sign-up asks for an email address
  (unique, unverified); "Forgotten your password?" emails a 30-minute link to the web app's `/reset-password` page
  (`WEB_APP_URL` on `lottery-api`, Resend SMTP); players from before add an address from the account screen; staff's
  temporary password remains the fallback. The Players page shows and searches addresses.
- **Players sign in with Google or Apple (owner, 2026-10-29, ADR 0020):** the sign-in screen offers both when the API
  lists them (`GET /v1/auth/providers`); an identity without an account is asked for its M-Pesa number and nothing else
  (`POST /v1/auth/sso/{provider}/signup`: no password, the number proved by the first top-up as always). Waits on the
  owner's client ids (`docs/runbooks/player-sign-in.md`): `GOOGLE_CLIENT_ID`, `GOOGLE_IOS_CLIENT_ID`, `APPLE_CLIENT_ID`,
  `APPLE_BUNDLE_ID` on `lottery-api`; until then the apps show neither button.
- **Identity review notices (owner, 2026-10-28):** whenever a player's ID submission starts waiting for a decision,
  the api emails `KYC_REVIEW_NOTIFY_TO` (itsivali@outlook.com on Render) a link that opens the back office at that
  submission (`internal/platform/mail`, job `kyc.review_notice`; `docs/runbooks/kyc-review.md`). The email names
  nobody. It is sent once `SMTP_HOST`, `SMTP_FROM`, `SMTP_USERNAME` and `SMTP_PASSWORD` point at a transactional
  provider (Resend or Brevo suggested); **to set in the Render dashboard**. Until then the notice is only logged.
- **Testing with real M-Pesa (owner, 2026-10-26):** top-ups from KES 1 (`TOP_UP_MIN_KES=1` on Render and in compose;
  the apps read the top-up and withdrawal bounds from `GET /v1/wallet`) and plays at KES 1 from 2026-10-26 (migration 00028). Before launch:
  set `TOP_UP_MIN_KES` and add an odds/price version with the launch price from the launch date. The apps point at
  `https://api.tajirika.co.ke` (repo variable `MOBILE_API_BASE_URL`; Render static site `lottery-web`).
- **Deposit excise (owner, 2026-10-26, ADR 0016): paid by the operator**; the player is credited in full and taxed only
  on winnings (from 2026-10-22 it was deducted from the deposit). Rate version to load: `deposit_excise` 5% `half_up` effective 2025-07-01 (owner's choice, based on the Finance
  Act 2025; to be confirmed with a tax adviser before production).
- Daraja: **sandbox only** (shortcode 174379). The owner's sandbox consumer key/secret and passkey work: a live KES 1
  push to the owner's test phone went pending (result 4999) and then paid (2026-10-22). They go into Render as
  `sync: false` variables, never into git.

## Slice 3 (C2B) decisions (owner, 2026-10-22)

- Sandbox C2B test shortcode **600992** (the Party A on the owner's Test Credentials page; changed from 600982 on 2026-10-24). STK Push stays on 174379. Production needs a real paybill later.
- **Account number = the player's registered phone number.** Only a *proved* phone matches (an unverified number can
  be claimed by several accounts); anything else goes to suspense for an operator.
- URLs `/hooks/c2b/{secret}/validate` and `/confirm` (C2B URL registration refuses paths containing "mpesa").
  Validation rejects unknown accounts, restricted players, deposits over a limit and bad amounts. A confirmation is
  stored and verified with Daraja's **Transaction Status API** before it is credited (ADR 0006), through the same
  credit path as STK top-ups.
- **Built (2026-10-24, ADR 0008):** validation, confirmation, Transaction Status verification, credit or suspense,
  operator actions (`lotteryctl payments suspense-credit | suspense-refund | c2b-recheck | c2b-register |
  c2b-simulate`), reconciliation of suspense. Paybill payments never prove a phone.
- **Needs from the owner:** the sandbox **initiator name** and its **security credential** (Daraja portal, Test
  Credentials page: the portal encrypts the initiator password into the credential). Then set `DARAJA_C2B_SHORTCODE=600992`,
  `DARAJA_INITIATOR_NAME`, `DARAJA_SECURITY_CREDENTIAL` on Render, deploy, run `lotteryctl payments c2b-register`, and
  simulate a payment to the test account (`c2b-simulate`).

## Owner items open at the end of the session

- **Paybill sandbox run:** URLs registered for 600992 (v1 and v2), payments simulated to the test account, and a
  Transaction Status request accepted with the owner's initiator credential, but Daraja has not called the hooks
  (they answer correctly from outside). Retry; if it persists, raise it with Safaricom.
- PR "release 1.0.0" (release-please) is **held** by the owner's decision; do not merge.
- The nightly job failed every night until #20 (2026-10-24): the property step tested the empty root package. It now
  passes (mutation: wallet 100%, ledger 97.7%, payments 89.7%).
- Trusted-proxy probe 2 from a second network; rotate the Render API key and the Daraja consumer secret.

## Next steps

1. **Test account re-created (2026-10-24):** 0796992457 signed up again, KYC approved by hand (owner-attested), and
   its first sandbox top-up (KES 10) was **credited and watched live over `GET /v1/payments/events`** (slice 5, #25,
   deployed), which proved the phone. Password in `~/.config/lottery-platform/test-account.env`.
2. Close the phase 2 checkpoint (C2B shelved, Buy Goods off until a till exists). Then phase 3 (game and tickets).
3. Owner: run trusted-proxy probe 2 from a second network; define the manual KYC review procedure and service level.
4. Later: password reset (STK proof + ID match), passkeys/TOTP, real Play Integrity / App Attest verifiers.

## Open questions for the owner

- **Payouts (phase 5) placeholders:** withholding on winnings 20% (`winnings_wht`), none on withdrawals
  (`withdrawal_wht` 0%); withdrawals KES 50–150,000, 3 a day; manual approval above KES 100,000. Confirm with a tax
  adviser and set them.
- **Settled 2026-10-25:** a prize first repays the winner's debt from a reversed top-up; public certificates name
  winning entries only, never players (the entry list stays regulator-only); withheld tax is remitted in numbered
  batches, prepared and remitted by two different people (`docs/runbooks/tax.md`); alerts go to a webhook with a
  heartbeat, and a failed reconciliation freezes payouts until acknowledged (`docs/runbooks/alerts.md`).
- **KRA:** how the return is filed (iTax upload, API, adviser); the remittance deadline (alerts use 72 h / 120 h
  placeholders); whether to keep payees' ID numbers (encrypted) for withholding certificates — only a hash is stored.
- **Email setup:** sign up with a transactional email provider, verify a sender address, and set `SMTP_HOST`,
  `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` and `SMTP_FROM` on `lottery-api` so the identity-review emails are sent.
- **Alerts setup:** create the channel and set `ALERT_WEBHOOK_URL`; create a healthchecks.io check (notifying a phone)
  and set `ALERT_HEARTBEAT_URL`; name who is on call and who may lift a payout freeze.

- Confirm the 5% deposit excise version with a tax adviser before production.
- Does GRA accept phone verification by M-Pesa confirmation instead of SMS OTP (ADR 0005), and manual identity checks
  instead of a registry lookup (ADR 0009)?
- Minimum verification deposit amount (suggested KES 10) and excise treatment on it.
- `internal/notifications` (SMS sender) is unused since ADR 0005: keep for a future channel or delete?
