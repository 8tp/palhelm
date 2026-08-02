---
title: Data channels
description: The core channels Palhelm uses to read and control a server, plus the optional item-grant bridge.
sidebar:
  order: 2
---

This page covers the three core channels Palhelm uses to talk to a Palworld dedicated
server: the official REST API, RCON, and the read-only save files. It also covers the
optional, local-only UE4SS item-grant bridge and maps each
Palhelm feature to the channel it draws from, and explains the read-only guarantee on
save files.

## The core channels

Palhelm never runs game logic itself. Everything it shows or does comes through one of
these core channels into the running server. Item grants are the one optional
exception and use the separate bridge described below.

### 1. Official REST API

The Palworld dedicated server exposes an official REST API, by default at
`http://palworld:8212/v1/api/*`, protected with HTTP basic auth as `admin` and the game
admin password. This is the primary channel. Palhelm uses it for:

- Server info and settings.
- Live player list with position, level, and ping.
- Server metrics: frame rate, frame time, player count.
- Optionally, Palworld 1.0's current world-actor snapshot for live player and
  base-Pal state.
- Announce, kick, ban, and unban.
- Save.
- Graceful shutdown with a wait time and message, and force stop.

Palhelm proxies this API server-side. The game admin password stays in the Palhelm
process and never reaches the browser.

The actor snapshot is treated more strictly than ordinary REST responses: it can be
large and can contain IP addresses, platform identifiers, exact positions, health, and
raw activity. Palhelm discards IP and platform-user fields during decode, applies hard
response/actor bounds, caches one typed generation in memory, and exposes only separate
allowlisted panel and Integration projections. More precisely, the raw generation is reduced
once to counts plus at most 2,048 sanitized useful actors; raw IDs/actions and the source actor
array are not cached. Exact actors expire server-side, while the Integration summary reads only
precomputed counts. It is a transient observation, not a
spawn table or a replacement for save-derived ownership.

### 2. RCON

Palhelm connects to Source RCON, by default at `palworld:25575`, using the same game
admin password. RCON backs the Console screen and the few actions the REST API does not
offer, such as teleporting a player. The vanilla RCON command set is small, and Palhelm
does not assume PalGuard or any mod is installed.

RCON has real limits worth knowing. Vanilla RCON has no whisper, and its `Broadcast`
command mangles spaces. Where the REST API can do the same job, Palhelm prefers it.

### 3. Save files, read-only

Palhelm mounts the server's `Saved/` directory and reads the save files: `Level.sav`,
`LevelMeta.sav`, and the per-player files under `Players/`. It parses them on the
save-sync interval and on demand to populate offline players, pals, guilds, and bases.

The `Players/` directory does not exist until the first player joins. Palhelm treats
that as a normal state, not an error.

### 4. Optional UE4SS item bridge

Item grants do not fit the vanilla REST, RCON, or save-file channels. When an
operator explicitly installs and enables the feature, Palhelm and a small UE4SS
server bridge exchange versioned request, result, and heartbeat files through a
local-only spool. The bridge mutates the online player's authoritative inventory;
the save parser remains read-only.

This channel is disabled by default and fails closed. A ready capability requires
a matching item catalogue and allowlist, a fresh heartbeat, the expected protocol,
and an exact game build marked production-validated by the bridge. Requests use
stable player and item IDs, bounded quantities, required reasons, idempotency, and
a durable audit ledger. Palhelm does not retry an uncertain mutation.

## Read-only guarantee on saves

Palhelm's save parser is decode only. It reads bytes and builds typed structs; it never
re-encodes or writes a save file. In-place save editing, such as giving items or pals,
is not used for item grants. The only component that writes into the save directory area is
the backup engine, which copies and archives save files and, on restore, swaps them in
through a guided flow that requires the server to be stopped. Parsing itself only reads.

## Which feature uses which channel

| Feature | REST API | RCON | Save files | Optional UE4SS bridge |
|---|---|---|---|---|
| Dashboard metrics and charts | Yes | | | |
| Live player list and positions | Yes | | | |
| Optional live Pal/base activity | Yes | | | |
| Announce, kick, ban, unban | Yes | | | |
| Save now | Yes | | | |
| Graceful shutdown and stop | Yes | | | |
| Console screen | | Yes | | |
| Teleport and other RCON-only actions | | Yes | | |
| Offline players, pals, guilds, bases | | | Yes | |
| Live map markers | Yes | | Yes | |
| Backups and restore | Yes | | Reads and copies files | |
| Allowlisted item grant | | | | Yes, when validated |

The live map is the one screen that blends channels. Live player positions come from the
REST API, while guild bases and other placement data come from the parsed save. The
backup engine uses the REST API to resolve the active world GUID and reads and copies
the save files themselves.

## What Palhelm cannot do through these channels

Some limits come from the channels, not from Palhelm:

- Restart is external. Palhelm can start and cancel a graceful shutdown countdown through
  the REST API, but once the server stops, no channel gives it a safe way to start the
  server again or observe it afterward. Host supervision handles restarts.
- Allow-list enforcement is not authoritative on vanilla servers. Palhelm reflects state
  it can read, but the vanilla server does not give it a supported way to control who may
  join.

These hedges are described in more detail in the panel and getting-started guides.
