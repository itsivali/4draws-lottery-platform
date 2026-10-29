# Tajirika — lottery platform

A real-money, mobile-money lottery for the Kenyan market. One play a spin, a few spins a day, prizes drawn from a
provably fair seed, money moving through M-Pesa and a double-entry ledger. This repository holds everything:

| Part | Where | What it is |
|---|---|---|
| API and workers | `cmd/api`, `cmd/worker`, `internal/` | Go modular monolith: accounts, wallets, M-Pesa top-ups and payouts, plays, draws, KRA tax, alerts, the back-office API |
| Database | `db/migrations`, `db/queries` | Postgres schema (goose migrations) and the SQL the modules use (sqlc) |
| Player app | `apps/mobile` | Flutter: the **web app** and the **Android and iOS apps** are one codebase |
| Back office | `apps/backoffice` | React: the staff application (players, identity review, payments, draws, payouts, tax, alerts, closures, finance, app metrics, staff) |
| Operator CLI | `cmd/lotteryctl` | Emergency and regulator tooling (`lotteryctl …`) |
| Design system | `design-system/lottery-platform/MASTER.md` | The "Sunset" look both apps follow |
| Decisions and runbooks | `docs/adr`, `docs/runbooks`, `docs/STATUS.md` | Why things are the way they are, and how to operate them |

Read `docs/STATUS.md` first in a new session: it is the running record of what is built, decided and pending.

---

## 1. Prerequisites

- **[mise](https://mise.jdx.dev)** pins every tool (`mise.toml`: Go, Node, Flutter, Java 17, sqlc, goose,
  golangci-lint, gitleaks, …). After cloning: `mise install`.
- **Docker** (with Compose) for the local stack and the integration tests.
- `jq` and `curl` for the seed and e2e scripts.
- For the Android app: an Android SDK (see the header of `scripts/dev-mobile.sh`). For iOS: a Mac with Xcode.
- `cloudflared`, so Safaricom's sandbox can call your laptop back (`mise use -g cloudflared@latest`, or the distribution's
  package). Needed for the default way of running the backend (section 2.1).
- A **Daraja sandbox app** on https://developer.safaricom.co.ke (free): its consumer key, consumer secret and the
  sandbox passkey go into `.env.sandbox` (section 2.1). Sandbox prompts reach real Safaricom numbers; no real money moves.

```sh
mise install     # the pinned toolchain
make setup       # git hooks (lefthook: gofmt, lint, gitleaks)
```

## 2. Running everything locally

Three scripts cover the whole stack. Each is idempotent: run it again to update.

### 2.1 Backend (API, database, cache, staff realm, M-Pesa sandbox)

**The default way to start the backend is sandbox mode**, so that every top-up and every M-Pesa-paid spin sends a
real STK prompt to the phone of the account, exactly as in production, against Safaricom's Daraja sandbox (no real
money). Once, create the credentials file:

```sh
cp .env.sandbox.example .env.sandbox      # gitignored; fill in your Daraja sandbox app's key, secret and passkey
```

Then, every time:

```sh
scripts/dev-backend.sh --sandbox  # start (or update) the stack with a tunnel for Safaricom's callbacks; waits until ready
scripts/dev-backend.sh --logs     # follow the api's logs
scripts/dev-backend.sh --down     # stop it and delete its data
```

The script prints `Daraja sandbox: callbacks reach https://….trycloudflare.com`: that address changes on every start
(it is a temporary Cloudflare tunnel), which is why the backend must always be started with this script in sandbox
mode rather than restarted by hand. Sandbox mode covers top-ups (STK push); withdrawals need sandbox B2C settings,
which are not configured, so the withdraw screen says they are unavailable.

Without `.env.sandbox` the stack can still run against a **mock M-Pesa** that approves every prompt by itself
(`scripts/dev-backend.sh`, no flag; `--seed` also creates the test player of section 3.2). Use it for automated
journeys and when you have no sandbox app; nothing reaches a phone.

What comes up (all in Docker, `docker-compose.yml` plus `docker-compose.sandbox.yml` in sandbox mode):

| Service | Address | Notes |
|---|---|---|
| `api` | http://localhost:8080 | The Go API with its workers in-process; migrations run at start |
| `postgres` | localhost:5433 | database `lottery`, user `lottery`, password `lottery` |
| `valkey` | localhost:6380 | rate limits, challenges, queues |
| `keycloak` | http://localhost:8081 | the **local staff realm** `lottery-staff`, a stand-in for Google/Apple sign-in |
| `daraja-mock` | http://localhost:8090 | the mock M-Pesa, used only when the stack runs without `--sandbox` |
| `drand-mock` | http://localhost:8093 | the randomness beacon the draws use |

The first start seeds the back-office staff (section 3.1) and, with `--seed`, the test player.

### 2.2 Frontends (player web app and back office)

```sh
scripts/dev-frontend.sh               # player web app, release build, http://localhost:8088
scripts/dev-frontend.sh --hot         # player web app with hot reload (press r)
scripts/dev-frontend.sh --backoffice  # the back office, http://localhost:5173
scripts/dev-frontend.sh --all         # both
```

The back office alone, by hand:

```sh
cd apps/backoffice
cp .env.example .env.local   # VITE_API_BASE_URL=http://localhost:8080 and the local staff realm
npm ci && npm run dev        # http://localhost:5173
```

The player web app alone, by hand:

```sh
cd apps/mobile
flutter run -d chrome --dart-define=API_BASE_URL=http://localhost:8080 --dart-define=BACKOFFICE_URL=http://localhost:5173
```

`BACKOFFICE_URL` is what the web app's "Staff? Open the back office" link points at; the mobile apps never show it.

### 2.3 Mobile apps

```sh
scripts/dev-mobile.sh             # Android: a phone over USB (adb reverse reaches the backend) or the emulator
scripts/dev-mobile.sh --emulator  # always the emulator (created the first time)
cd apps/mobile && flutter run -d ios --dart-define=API_BASE_URL=http://localhost:8080   # iOS simulator, on a Mac
```

Release builds must name the server: `--dart-define=API_BASE_URL=https://…` (plain http is refused outside
localhost; a build without a server refuses to start rather than guess one).

## 3. Accounts

Everything below exists **only in the local stack** (compose, the Keycloak realm file, the seed scripts). Nothing here
works against Render or any other deployment.

### 3.1 Back-office staff (http://localhost:5173)

Staff sign in with their **email address and password** (ADR 0019). The seeded members all start with the temporary
password **`local-dev-staff-2026`**, and the back office asks for a new one at the first sign-in. Alternatively, "Sign
in with the local staff realm" opens Keycloak, where the same people sign in with username (`ada`, `alice`, …) and
password **`local-dev-staff`**.

| Email | Role | Notes |
|---|---|---|
| `ada@staff.local` | Administrator | the bootstrap admin (`STAFF_BOOTSTRAP_ADMIN` / `STAFF_BOOTSTRAP_PASSWORD` in compose) |
| `alice@staff.local` | Administrator | |
| `bob@staff.local` | Staff | |
| `kim@staff.local` | Staff | the identity reviewer the seed script uses |
| `paula@staff.local` | Staff | |
| `otis@staff.local` | Staff | |
| `tara@staff.local` | Staff | |
| `tom@staff.local` | Staff | |
| `sam@staff.local` | Staff | |

Roles: **Staff** work every operational queue (players, identity review, payments, draws, payouts, KRA tax, alerts,
account closures) and see Finance; **Administrators** also manage staff (invite, roles, temporary passwords, disable) and see
App metrics. Two-person rules still need two different people (two sign and two approve each draw; the one who
remits a tax batch is not the one who prepared it). Re-run `scripts/seed-staff.sh` any time; existing members are
left alone. `scripts/seed-staff.sh token kim` prints a session token for curl.

### 3.2 Test player (player web app and mobile apps)

Created by `scripts/dev-backend.sh --seed`, already verified so it can play, top up and withdraw at once:

| Phone number | Password | State |
|---|---|---|
| `0712345678` | `correct horse battery` | ID verified (approved as `kim`), phone proved, wallet credited by the M-Pesa mock |

To receive real prompts, **create an account with your own Safaricom number** from the app's sign-up screen (the
sandbox cannot prompt the seeded number). Its identity submission then waits in the back office (Identity review) for
`kim` or anyone else to approve; its first credited top-up proves the phone, and from then on plays, top-ups and the
"Pay KES 1 with M-Pesa and spin" button all send prompts to that phone. In mock mode the prompt is approved for you.

### 3.3 Infrastructure

| What | Credentials |
|---|---|
| Postgres (compose) | `postgres://lottery:lottery@localhost:5433/lottery` |
| Keycloak admin console (compose) | `admin` / `admin` at http://localhost:8081 (the realm is imported from `deploy/keycloak`) |
| Daraja sandbox | your own app's consumer key, secret and passkey in `.env.sandbox` (never production ones); shortcode `174379` |

### 3.4 Deployed (Render)

| Service | Address | Sign-in |
|---|---|---|
| `lottery-api` | https://api.tajirika.co.ke | the API |
| `lottery-web` | https://tajirika.co.ke (www redirects there): the player web app | players' own accounts |
| `lottery-backoffice` | https://backoffice.tajirika.co.ke: the back office | **admin@tajirika.co.ke** (shown as "Administrator") is the bootstrap administrator; their first password is `STAFF_BOOTSTRAP_PASSWORD` (a Render secret, changed at the first sign-in). Google and Apple sign-in work once the client ids are configured (`docs/runbooks/back-office.md`). |

The domain `tajirika.co.ke` is registered with Safaricom, whose DNS holds the records (an A record for the bare domain,
CNAMEs for `www`, `api` and `backoffice` to the services' onrender.com addresses, which keep working). Deployment is
described by `render.yaml` (a Blueprint). Secrets are set in the Render dashboard, never in the
repository; `docs/ci-secrets.md` lists what CI needs.

Every identity submission emails **itsivali@outlook.com** (`KYC_REVIEW_NOTIFY_TO`) a link into the back office once
the `SMTP_*` variables name a transactional email provider (`docs/runbooks/kyc-review.md`); until then the api only
logs the notice. Locally the notice is logged by the `api` container (`docker compose logs api | grep "email not sent"`).

## 4. Tests and checks

```sh
make ci                                # exactly what CI runs for Go: lint, unit, integration (Docker), coverage
make test                              # Go unit tests
make test-int                          # Go integration tests (Postgres and Valkey in testcontainers)
make e2e                               # the whole journey against the compose stack (scripts/e2e.sh)
make gen && make gen-check             # regenerate sqlc / oapi-codegen and fail if anything changed
make openapi-lint                      # api/openapi.yaml

cd apps/backoffice && npm test && npm run lint && npm run typecheck
cd apps/mobile && flutter test         # includes golden images; --update-goldens after an intended visual change
```

## 5. Where to look next

- `docs/STATUS.md`: what is built, every owner decision with its date, what is pending.
- `docs/adr/`: the decisions (money as whole KES, idempotency, manual KYC review, personal draw periods, two staff
  roles and password sign-in, …).
- `docs/runbooks/`: operating the back office, payments, payouts, draws, tax, alerts, incidents, secrets and keys.
- `docs/checkpoints/`: the phase checkpoints.
- `CONTRIBUTING.md` and `LICENSE` (all rights reserved).
