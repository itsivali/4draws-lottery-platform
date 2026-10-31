# 0026. The back office helps with audits: a filtered, exportable activity log and a per-player timeline

Status: accepted
Date: 2026-10-31

## Context
The owner asked (2026-10-31) that the back office "help with audits": an auditor, or the owner, must be able to
answer "what happened to this player / this money / on this day, and who did it" quickly, and take the evidence away
as files. The audit log (`audit.audit_log`, append-only since migration 00007) already held every fact, but the back
office could only search it by a text fragment and page through it, with staff shown as `staff:<uuid>`; a player's
story was spread over the players, plays, top-ups, payouts, identity and closures pages; and the only files that left
the back office were the KRA batch files and, with ADR 0023, the finance and ledger exports.

## Decision
- **Filters on the activity log.** `GET /v1/staff/activity` takes, besides `q` and `actor_type`: `from` and `to`
  (Nairobi days, both inclusive), `action` (a prefix: `payment.` is every payment event), and exact `entity_type`,
  `entity_id` and `actor_id`. Migration 00043 adds the indexes the filters need - `(actor_id, id)`,
  `(action text_pattern_ops, id)`, `(at)`, `(entity_id, id)` and `((details->>'user_id'), id)` - and touches nothing
  else: the append-only triggers and the table's grants stay as they were. The back office page carries the filters
  as fields, shows staff by name (from `GET /v1/staff/members`, admins only, as the page is), renders details as
  readable chips, and links each id to the page that shows it.
- **Exports are hashed and audited, like the finance files.** `GET /v1/staff/activity/export` takes the same
  filters and streams a CSV (id, at_eat, actor_type, actor_id, actor_name, action, entity_type, entity_id,
  details_json) from one repeatable-read snapshot, hashed before the first byte goes out, so `X-File-SHA256` and
  `X-File-Rows` are headers; the export is recorded as `activity.exported` with the filters, the row count and the
  hash, so an auditor's file can be tied to the log it came from. More than 100,000 records is refused
  (`too-many-rows`): narrow the filters rather than wait for a file nobody can open.
- **One player's timeline.** `GET /v1/staff/players/{id}` returns the account as the player list shows it, the
  wallet (balance, debt and the last 50 movements), the last 20 top-ups, plays (the count and the last 20), the last
  20 payouts, the identity history (submissions' outcomes without the personal data they carry, the last 10
  sign-ins with the device's attestation verdict, an open closure request), the deposit limits and any time-out or
  self-exclusion in force (from the rg service, now one of the staff services), and the last 50 audit records about
  the player - as actor, as entity, or named in the details. The reads are fixed SQL on the staff pool
  (`internal/oversight/player.go`). `GET /v1/staff/players/{id}/export` is the **player pack**: every row of every
  list in one CSV with shared columns (section, id, at_eat, kind, status, amount_kes, reference, details_json),
  hashed and audited as `identity.player_exported`, for an auditor or a data-subject request.
- **Unmasking stays audited.** Opening a timeline records `identity.player_viewed` (one event) and, through the KYC
  service, `kyc.unmask` for the name and ID number, as the player list does. The pack records the export instead of
  a view. Nothing in the timeline or the pack carries a date of birth or anything the player list does not show.
- In the back office the Players page opens the timeline in a sheet from an *Open* button or a `?player=<id>` link
  (the URL mirrors the open player, so the link can be shared), with the existing actions (suspend, reinstate,
  temporary password) and the pack's download inside it.

## Consequences
- Five more indexes on a table that is written on every action. They are small (ids and short text) and the writes
  are inserts only; the cost is accepted for the reads an audit needs.
- Exports read a snapshot twice (once to hash, once to send). For the activity log that is bounded by the row cap;
  the player pack is built in memory, since one player's history is small.
- `actor_id` is matched exactly: a staff member is `staff:<uuid>`, a player their user id. The page's search box
  (`q`) still matches fragments.
- The action prefix list offered by the page is a convenience; any prefix typed into the API works.
