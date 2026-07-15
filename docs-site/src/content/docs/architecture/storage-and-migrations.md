---
title: Storage and migrations
description: The single SQLite file, what its tables hold, metrics retention windows, and the ten schema migrations.
sidebar:
  order: 4
---

This page covers where Palhelm keeps its data: one SQLite file, its main table areas,
how long metrics are kept, and how the schema is versioned through ten ordered
migrations.

## One SQLite file

All Palhelm state lives in one SQLite database inside the mounted data volume. The
driver is `modernc.org/sqlite`, a pure-Go implementation, so no CGO or external database
server is involved. The same volume also holds backups, downloaded map tiles, downloaded
Pal icons, and the runtime-downloaded save decompressor.

## Table areas

The schema groups into a few areas:

- Metrics. `metrics` holds raw samples keyed by timestamp: frame rate, frame time, and
  player count. `metrics_rollup` holds one-minute aggregates with average, minimum, and
  maximum for each value.
- Players and sessions. `players` holds one row per known player, online or offline,
  including identity, level, guild, position, playtime, ban and allow-list flags, and
  progress counters. `sessions` records join and leave times.
- World data from the save. `guilds`, `guild_members`, `bases`, and `pals` are refreshed
  by the save-sync poller. `world_state` holds one row of parse metadata: the world day,
  when the last parse ran, how long it took, the counts it produced, and the skipped and
  drift counters from the parser.
- Paldeck observations. `player_paldeck` stores normalized species capture counts and
  unlock flags decoded from authoritative player `RecordData`; `player_paldeck_state`
  distinguishes unavailable, complete, and defensively truncated observations.
- Operational logs. `events`, `console_log`, and `saved_commands` back the events feed,
  the console history, and saved console commands.
- Aggregate Game Data diagnostics. `game_data_activity` stores FPS, actor counts, worker
  activity categories, and exact-link coverage only. It never stores actor identity,
  names, health, guilds, raw actions, or locations; samples age out after 30 days.
- Backups and access. `backups` records each archive. `whitelist` holds allow-list
  entries. `api_keys` holds Integration API keys, stored as SHA-256 hashes, never
  plaintext. `kv` is a small key-value table that, among other things, tracks the schema
  version.

## Metrics retention

Metrics are kept on two windows so charts stay dense recently and cheap over the long
run:

- Raw samples are kept for 24 hours.
- One-minute rollups are kept for 30 days.

Older raw samples age out once they have been rolled up, and rollups older than the
30-day window age out in turn.

## The migration runner

Schema changes go through a small embedded migration runner. Migration files live in
`backend/internal/store/migrations/` and are named `NNN_name.sql`. They are applied in
order inside `store.Open()`. The current schema version is tracked in the
`kv.schema_version` row.

The runner is careful about concurrency and safety:

- Each migration runs in its own `BEGIN IMMEDIATE` transaction, and re-reads the version
  inside the transaction so two processes cannot apply the same migration twice.
- A brand-new database is opened directly at the newest schema. An existing database is
  upgraded one migration at a time.
- It fails closed. If a database reports a version newer than the binary understands, the
  runner refuses to open it rather than risk operating on a schema it does not know.

This means an older binary will not open a database that a newer release has already
migrated: it sees a schema version above the newest migration it knows and refuses to
start. To roll back past a migration, restore the pre-update copy of the data volume.
See [Updating Palhelm](/getting-started/updating/) for the full rollback procedure.

## The ten migrations

| File | Purpose |
|---|---|
| `001_init.sql` | Initial schema: metrics and rollups, players, sessions, events, saved commands, console log, key-value store, guilds and members, bases, pals, world state, allow list, and backups. Seeds `schema_version`. |
| `002_api_keys.sql` | Adds the `api_keys` table that backs Integration API bearer tokens. Stores a public key id, a SHA-256 hash of the full key, a label, and timestamps. |
| `003_remove_pending_identity_players.sql` | Removes placeholder player and session rows that used a `none` identity, so a ghost player never appears. |
| `004_pal_party_box.sql` | Adds party and storage-box placement columns to `pals`: whether a pal is in the active party, its party slot, and its box page and slot. |
| `005_pal_owner_provenance.sql` | Adds an `owner_source` column to `pals` that records how ownership was determined, with a check constraint limiting it to known values. Backfills older rows conservatively. |
| `006_player_progress.sql` | Adds progress counters to `players`: total captures, unique pals captured, and Paldeck entries unlocked. |
| `007_pal_instance_details.sql` | Adds per-pal detail columns: HP, gender, the four talent values, passive skill ids, and equipped skill ids. |
| `008_pal_base_workers.sql` | Adds a `base_id` column to `pals` so pals assigned to a base can be linked to it. |
| `009_game_data_activity.sql` | Adds aggregate-only Game Data activity/health samples for bounded operator diagnostics. |
| `010_player_paldeck.sql` | Adds authoritative per-player species capture/unlock observations and their availability, timestamp, and truncation state. |

Migrations 004 through 010 grew the pal/player records and aggregate diagnostics as the panel's player-view
and pal-box screens matured. Each one is additive, so upgrading is a matter of pulling a
newer image and restarting.
