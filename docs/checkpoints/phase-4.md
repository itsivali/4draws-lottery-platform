# Phase 4 — Draws: checkpoint

Status: COMPLETE in code (2026-10-25); the first live draw signed and approved by two officials is still to come.

## Owner decisions (2026-10-24)
- **Draws:** daily, weekly, monthly and quarterly. **A play day's chances enter all four**: that day's daily draw and
  the week's (Monday–Sunday), month's and quarter's draws that contain the day. One entry per chance.
- **Prizes are a share of the period's stakes** (`pool_share`): each draw's pool is a percentage of the stakes sold in
  its period, split across ranks. The operator never owes more than it took in. **Values are placeholders** until the
  owner sets them: daily 30%, weekly 10%, monthly 5%, quarterly 5% of stakes (50% in all); ranks 50/30/20% of the pool.
- **Timing:** entries freeze at **00:05 Nairobi** after the period ends; the draw uses the first drand round published
  at or after **09:00 Nairobi** that day (owner, 2026-10-24: changed from 00:30 so the officials can sign in working
  hours). Daily for the previous play day; weekly on Monday, monthly on the 1st, quarterly on the 1st of
  January/April/July/October, for the period just ended.
- **Sign-off:** two different company officials sign the frozen entry list (`signed`) and approve the result
  (`approved`), through `lotteryctl` until the back office (phase 6). The GRA official's step is added when GRA names one.

## Plan (written before coding)

Rules across every slice:
- **Explicit state machine** (`open → frozen → signed → conducted → approved → paid`, or `voided` and re-run as a new
  row). Every transition is one transaction that checks the current state; the one-time seed and the append-only
  entries and results are already database rules (phase 1).
- **Nobody can know the outcome before it is fixed.** At freeze the entry list is numbered and hashed, the company
  commits to a fresh secret seed (`seed_commit`), and the drand round is fixed: the first round published after the
  signing window closes. Winners come from SHA-256(drand randomness ‖ revealed company seed ‖ entry-list hash ‖ draw id),
  drawn with the same rejection sampling as spins. The company cannot predict drand; drand cannot know the entries or
  the company seed; changing the entries changes the hash both officials signed.
- **The signatures must be complete before the drand round is published.** Signing after it would let a signer see the
  result and void a draw they dislike. A draw not signed by both officials before its round (09:00) is voided and
  re-run with a new round.
- Beacons are verified (BLS signature against the pinned chain key, randomness = SHA-256 of the signature), never just
  fetched.
- Prize amounts: pool = share × stakes of the period (whole KES, the prize structure's rounding), rank amounts =
  pool × rank share; what rounding leaves stays in the pool account. Funding is a ledger posting from
  `STAKES_REVENUE` to the draw type's `PRIZE_POOL_*` at approval; payouts (B2C, withholding tax) are phase 5.
- **One prize per player per draw** (owner, 2026-10-24). Winners are drawn entry by entry from the random stream; an
  entry of a player who already won is skipped, so more entries still mean better odds of winning once. With fewer
  players than ranks, the remaining ranks are not awarded and their share stays in the pool.

### Slice 1 — drand client
`internal/draws/drand`: pinned quicknet chain (hash, public key, genesis, period), round ↔ time, BLS verification
(kyber, `bls-unchained-g1-rfc9380`), fetch through several relays with fallback, every beacon verified whichever relay
served it. Tests on real recorded beacons and tampered ones.

### Slice 2 — Draw lifecycle
Migration (prize-structure placeholders, signing window), `internal/draws` service: open (scheduled), freeze (entries
from tickets and spins, hash, seed commitment, drand round), sign (two different officials, before the round),
conduct (verified beacon, winners), approve (two officials; pool funding posting), void + re-run. River jobs for the
schedule; `lotteryctl draws sign|approve|void|show`. Integration tests for every transition and refusal.

### Slice 3 — Public results and checkpoint
`GET /v1/draws` (schedule, results with drand round, beacon, entry-list hash, revealed seed), `GET /v1/draws/{id}/entries/mine`,
a public draw verifier in `tools/verifier`, reconciliation of pools, monitoring of missed windows, checkpoint report.

Detailed plan (2026-10-25, before coding):
- **Public reads, no token:** `GET /v1/draws` (newest first, `limit` ≤ 100) and `GET /v1/draws/{id}`: type, period,
  status, freeze time, entry count, entry-list hash, seed commitment, drand round and its time, revealed seed, beacon,
  certificate hash, results (rank, amount, entry number), void reason and re-run link. Officials' names are staff
  personal data and stay internal (the public sees each step's time and attestation hash). Rate-limited per address.
- `GET /v1/draws/{id}/certificate` serves the certificate **byte for byte as hashed**, so `sha256sum` of the download
  equals `certificate_hash` (re-encoding the JSON in a response would change the bytes).
- `GET /v1/draws/{id}/entries/mine` (signed in): the player's own entries (entry number, play day, ticket), their
  user id (as it appears in the certificate) and any prize. Filtered by the caller (IDOR test).
- **Verifier** (`tools/verifier draw`): independent of the server's code (it uses kyber for BLS, nothing from
  `internal/`). From the certificate it checks the beacon against the pinned quicknet key, the seed against its
  commitment, the pool and prize amounts, and that every winner is the pick the algorithm gives in rank order. With the
  full entry list (`lotteryctl draws entries`, for the regulator) it also checks the entry-list hash, the entry count and
  every skipped pick (one prize per player). Golden certificate built from a real recorded beacon, shared by the
  server's and the verifier's tests (like the spin vectors).
- **Reconciliation** (daily job and `lotteryctl draws reconcile`, runs kept append-only in
  `draws.reconciliation_runs`): certificate hash, seed vs commitment, entry-list hash and count recomputed, winners and
  amounts recomputed from the certificate and the stored entries (beacon verified again, offline), results rows ⇔
  certificate, prizes ≤ pool, one funding journal of exactly the pool for each approved draw and none otherwise, and each
  `PRIZE_POOL_*` balance = what approved draws of that type funded (payouts subtract it in phase 5).
- **Missed windows:** each schedule run reports draws that are late — open 30 minutes past their freeze time, frozen
  with under an hour of signing left, signed 30 minutes past their round, conducted but unapproved for a day — as
  `draw overdue` errors in the logs (Render), and `lotteryctl draws overdue` lists them.

## Results

### Slice 1 (drand client)
- `internal/draws/drand`: quicknet pinned (chain hash, G2 public key, genesis, 3 s period), `RoundAt`/`TimeOf`,
  `Verify` (BLS on G1 over SHA-256 of the round, randomness = SHA-256 of the signature), `Fetch` through several relays
  (api, api2, api3, Cloudflare), refusing before the round's time and refusing any beacon that does not verify.
- Tests on three real recorded beacons; refused: another round's signature, a relabelled round, swapped randomness,
  a wrong key, bad hex; a lying relay is skipped for an honest one; outages and not-yet-published rounds. 93.4%.
- Verification uses kyber's `sign/bdn` scheme on G1 (its single-signature check is plain BLS; the older `sign/bls`
  package is deprecated for a rogue-key weakness that only concerns aggregation).

### Slice 2 (draw lifecycle)
- Migration `00021`: prize structures as a share of stakes (placeholders: daily 30%, weekly 10%, monthly 5%,
  quarterly 5%; ranks 50/30/20%, rounded down); sign-offs one row per official (append-only); what each state must carry
  (CHECK); a trigger that allows only forward transitions and refuses changing a fixed draw; the certificate stored as
  `json` so it is published byte for byte as hashed (`jsonb` reorders keys: caught by the test that rehashes it).
- `internal/draws`: `Schedule` (every 5 minutes in the worker: open the current periods, freeze at 00:05, void and
  re-run draws not signed before their round, conduct signed draws once their round is out), `Freeze` (entries per
  chance, entry-list hash, encrypted company seed and its commitment, round = first at or after 09:00, or the next
  09:00 at least an hour away), `Sign`/`Approve` (two different officials; signing closes when the round is
  published), `Void` (frozen or signed, before the round; re-runs with a new seed and round), `Conduct` (verified
  beacon fetched outside the transaction; one prize per player; certificate with everything needed to recompute),
  approval posts `ledger.PrizePoolFunding`; the game reconciliation now counts pool funding.
- `lotteryctl draws list | show | sign | approve | void`; runbook `docs/runbooks/draws.md`.
- Tests: periods, freeze time, uniform picks; the full journey (entries 2 + 10 + 1 from tiers and a lucky day, hash
  recomputed, two signatures, nothing drawn before 09:00, winners recomputed from the certificate, pool 210 of 700
  funded at approval); unsigned → voided and re-run with the next 09:00 round; official void before but not after the
  round; one prize per player; empty day voided; drand down → waits; the database refuses skipped and backward
  transitions, changes to a fixed draw and deleted entries. Coverage 86%.

### Slice 3 (public results, verifier, reconciliation, monitoring)
- Public, no token, rate-limited per address: `GET /v1/draws`, `GET /v1/draws/{id}` (officials' names and entry
  user ids left out), `GET /v1/draws/{id}/certificate` (the stored bytes, so the download hashes to
  `certificate_hash`; also sent as `X-Certificate-SHA256`). Signed in: `GET /v1/draws/{id}/entries/mine` (own entries,
  own user id, own prizes; another player sees none of them).
- Winner selection moved into a pure `Award`, used by `Conduct`, the reconciliation and a golden test; the golden
  certificate (`internal/draws/testdata`) is built from the real recorded quicknet round 1000000 and includes a skipped
  repeat winner.
- `tools/verifier draw`: independent of `internal/` (kyber for BLS, its own key, pick and rounding). Checks the hash,
  the beacon against the pinned key, the seed, pool and prizes, every winner in rank order; with the entry list
  (`lotteryctl draws entries`) also the entry-list hash, count and every skipped pick. Refuses: swapped or reordered
  winners (also without the entry list), inflated prizes or pool, changed stakes, another seed, randomness, round or
  chain, a repeat winner, a rank given to the wrong player, a re-encoded certificate, a doctored entry list.
- Reconciliation (`draws.reconciliation_runs`, migration 00022, daily job `draws.reconcile`, `lotteryctl draws
  reconcile`): entry numbering and hash, certificate hash, certificate ⇔ draw, beacon (offline, through the `Beacon`
  interface), pool and winners recomputed against the prize structure in force, result rows ⇔ certificate, prizes ≤
  pool, funding journals per draw, `PRIZE_POOL_*` balances. Tests: clean at every step; catches a funded open draw, an
  entry added after freeze, a rewritten certificate (prize and beacon), an extra result row, a swapped seed, funding
  of an unapproved draw, the wrong funding amount, a prize structure slipped in behind a draw, a pool that is not the
  share of the stakes.
- Monitoring: each schedule run logs `draw overdue` for open draws 30 minutes past freeze, frozen ones under an hour
  before their round, signed ones 30 minutes past it, conducted ones unapproved a day later; `lotteryctl draws overdue`.
- Hand mutations: the verifier's later-winner check and its entry-list skip check, and the reconciliation's funding
  amount check, each survived at first; tests were added and all three are now killed.
- Also fixed: the game tampering test swapped the revealed seed for a fixed one, which reproduces the stored slot one
  time in 12 (it failed once in CI here); it now picks a seed that moves the slot.
- Coverage: draws 85.3%, overall 86.4%; `make ci` green.

## Checkpoint (2026-10-25)

Phase 4 is complete in code: four draw types over each play day's chances, entries frozen and hashed, a committed
company seed and a future drand round fixed at freeze, two officials signing before the round, verified beacons,
one prize per player, certificates anyone can check with `tools/verifier draw`, pools funded on approval and reconciled
daily, late steps reported. Open for the owner: the prize structure values (placeholders), the officials who will sign
the first live draw, whether winners' user ids may stay in the public certificate, and where alerts should go.
Payouts of prizes (`approved → paid`) are phase 5.
