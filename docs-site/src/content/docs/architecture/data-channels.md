---
title: Data channels
description: The three ways Palhelm reads and controls a Palworld server, and which feature uses which.
sidebar:
  order: 2
---

This page covers the three channels Palhelm uses to talk to a Palworld dedicated
server: the official REST API, RCON, and the read-only save files. It also maps each
Palhelm feature to the channel it draws from, and explains the read-only guarantee on
save files.

## The three channels

Palhelm never runs game logic itself. Everything it shows or does comes through one of
three channels into the running server.

### 1. Official REST API

The Palworld dedicated server exposes an official REST API, by default at
`http://palworld:8212/v1/api/*`, protected with HTTP basic auth as `admin` and the game
admin password. This is the primary channel. Palhelm uses it for:

- Server info and settings.
- Live player list with position, level, and ping.
- Server metrics: frame rate, frame time, player count.
- Announce, kick, ban, and unban.
- Save.
- Graceful shutdown with a wait time and message, and force stop.

Palhelm proxies this API server-side. The game admin password stays in the Palhelm
process and never reaches the browser.

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

## Read-only guarantee on saves

Palhelm's save parser is decode only. It reads bytes and builds typed structs; it never
re-encodes or writes a save file. In-place save editing, such as giving items or pals,
is an explicit non-goal. The only component that writes into the save directory area is
the backup engine, which copies and archives save files and, on restore, swaps them in
through a guided flow that requires the server to be stopped. Parsing itself only reads.

## Which feature uses which channel

| Feature | REST API | RCON | Save files |
|---|---|---|---|
| Dashboard metrics and charts | Yes | | |
| Live player list and positions | Yes | | |
| Announce, kick, ban, unban | Yes | | |
| Save now | Yes | | |
| Graceful shutdown and stop | Yes | | |
| Console screen | | Yes | |
| Teleport and other RCON-only actions | | Yes | |
| Offline players, pals, guilds, bases | | | Yes |
| Live map markers | Yes | | Yes |
| Backups and restore | Yes | | Reads and copies files |

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
