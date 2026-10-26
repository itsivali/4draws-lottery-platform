# Load test

`go run ./loadtest` drives the player journey with many players at once against a **local or staging** stack and then
checks that the money still adds up: at most 4 plays a player a day, a replayed idempotency key charged once, every
wallet equal to what was credited minus the plays. It exits 1 on any failed journey or broken invariant.

```sh
docker compose -f docker-compose.yml -f loadtest/compose.loadtest.yml up -d --wait   # trusts X-Forwarded-For: local only
go run ./loadtest -players 100 -concurrency 25 -spoof-ip
```

`-spoof-ip` gives each simulated player its own address so per-address limits (10 sign-ups an hour) apply per player
as in production; without it, only 10 players can sign up from one machine an hour.

## Results

2026-10-26, local compose stack on a developer laptop (api with workers in process, Postgres 16, Valkey 8, mocks):
100 players, 25 at a time, 29 s; no failed journeys; 700 play attempts → 400 plays (exactly 4 a player), the rest
refused (`daily-limit`, or `request-in-progress` for the concurrent replay). p50/p95/p99: sign-up 780 ms / 1.36 s /
1.37 s (argon2id), KYC 323 ms / 898 ms / 994 ms, top-up 191 ms / 283 ms / 764 ms, play 851 ms / 1.64 s / 1.65 s (7
plays a player at the same instant: they queue on the player's day row by design). Invariants held. Render's free
plan is much smaller; measure there before launch (phase 8).
