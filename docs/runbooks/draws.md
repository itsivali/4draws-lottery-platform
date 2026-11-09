# Runbook: draws

Daily, weekly, monthly and quarterly draws (phase 4, `docs/checkpoints/phase-4.md`). The worker runs the schedule every
5 minutes; officials sign and approve with `lotteryctl` (back office later).

## Every morning

1. After 00:05 Nairobi the period's draws are **frozen**: `go run ./cmd/lotteryctl draws list`.
2. Two different officials each run, **before 09:00** (the drand round is published then):

   ```sh
   go run ./cmd/lotteryctl draws show -draw <id>        # entries, entry_list_hash, seed_commit, drand_round_at
   go run ./cmd/lotteryctl draws sign -draw <id> -operator <your back-office email>
   ```

   A draw not signed by two officials before its round is voided and re-run automatically with the next 09:00 round.
3. After 09:00 the worker conducts it with the verified drand beacon. Check the results and certificate (`draws show`),
   then two officials approve; approval moves the pool into `PRIZE_POOL_*`:

   ```sh
   go run ./cmd/lotteryctl draws approve -draw <id> -operator <your back-office email>
   ```

## Voiding

Only a frozen or signed draw, and only before its round: `draws void -draw <id> -reason "<why>" -operator <your back-office email>`
(needs the api's configuration: the re-run's seed is encrypted with `DATA_KEY`). A conducted draw cannot be voided.

## When drand is down

The draw stays signed and the worker tries every 5 minutes (relays: api, api2, api3.drand.sh, Cloudflare). Beacons are
verified against the pinned quicknet key whichever relay serves them.

## Late steps (`draw overdue`)

Every schedule run logs one `draw overdue` error per late draw, and `go run ./cmd/lotteryctl draws overdue` lists them:

| Status | Reported when | Do |
|---|---|---|
| open | 30 minutes past its freeze time (00:05) | The worker is not running: check the service and its logs. |
| frozen | under an hour before its round, not yet signed by two officials | Get two officials to sign now, or it is voided and re-run with the next 09:00 round. |
| signed | 30 minutes past its round, not conducted | drand unreachable or the worker down (see below). |
| conducted | a day past its round, not approved | Two officials review the certificate and approve. |

## Reconciliation

The worker reconciles draws daily (`go run ./cmd/lotteryctl draws reconcile` runs it now; runs are kept in
`draws.reconciliation_runs`). For every frozen or later draw it recomputes the entry-list hash from the stored entries,
and for conducted ones the certificate hash, the seed against its commitment, the beacon (offline), the pool and the
winners, and checks the result rows, that prizes stay within the pool, one funding journal of exactly the pool for each
approved draw (none otherwise), and each `PRIZE_POOL_*` balance against the approved pools of its type. Any mismatch
is a bug or tampering: stop approving draws, keep the evidence and investigate before anything is paid.

## Public verification

The protocol and the verifier are published at https://github.com/itsivali/tajirika-fairness (MIT): the one place to
point players, auditors and the regulator. Anyone can check a conducted draw without trusting the server:

```sh
curl -s https://<api>/v1/draws/<id> | python3 -m json.tool          # certificate_hash, drand round, results
curl -s https://<api>/v1/draws/<id>/certificate -o cert.json        # the exact bytes that were hashed
go run ./tools/verifier draw -certificate cert.json -hash <certificate_hash>
```

The verifier checks the beacon against the pinned quicknet key, the seed, the pool and prizes, and that each winner is
the algorithm's pick. Certificates name winning **entries**, never players (owner's decision, 2026-10-25): that no
player won twice, and that each skipped pick landed on a player who had already won, are confirmed only with the full
entry list, which is not public: export it for the regulator with `go run ./cmd/lotteryctl draws entries -draw <id> > entries.jsonl` and pass
`-entries entries.jsonl`. Players see their own entries and prizes with `GET /v1/draws/{id}/entries/mine`. A
certificate issued before winners were left out (it names them) cannot be changed, since it is hashed and signed off, so
the public endpoint withholds it (`403 certificate-withheld`); read it from the database for the regulator.
