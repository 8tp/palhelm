---
title: Commands
description: Every slash command the bot registers, grouped by area, with arguments and which ones need the admin role.
sidebar:
  order: 3
---

This page lists every slash command the bot registers, grouped by area, with each command's arguments and whether it needs the admin role. There are 32 commands in total. Five of them are admin only and are marked **ADMIN**.

Arguments in `<angle brackets>` are required. Arguments in `[square brackets]` are optional. Names with autocomplete resolve players and pals as you type.

Admin commands are limited to members who hold the role set in `ADMIN_ROLE_ID`, and they act on the game server. See the [Safety model](/bot/safety-model/) for exactly what admin access requires.

## Server

| Command | Arguments | What it does |
|---|---|---|
| `/status` | | Server name, state, version, uptime, online count, world day, and FPS. |
| `/metrics` | | FPS, frame time, player count, world day, uptime, and base-camp counts. |
| `/map` | `[layer]` | World map image with guild bases and live players plotted. Degrades to a text base list when tiles are absent. |
| `/guilds` | | Guilds with their member and base counts. |
| `/help` | | Categorized directory of every command. |

## Players

| Command | Arguments | What it does |
|---|---|---|
| `/players` | | Who is online right now. |
| `/player` | `<name>` | Player profile card plus top pals. |
| `/compare` | `<a> <b>` | Side-by-side comparison of two players. |
| `/leaderboard` | `[category]` | Rankings by level, playtime, pal count, rare pals, or guild. Switchable with a select menu. |
| `/profile` | `link \| status \| unlink` | Link your Discord user to a Palworld player so "self" queries work. |

## Pals

| Command | Arguments | What it does |
|---|---|---|
| `/pal` | `<pal> [player]` | Inspect one owned pal: work suitability, stats, skills, and placement. |
| `/pals` | `<name>` | A player's pals rendered as an icon-grid image. |
| `/box` | `<name> [page]` | Browse a player's pal storage box with previous and next buttons. |
| `/whohas` | `<pal>` | Find current owners of a species. |
| `/rare` | `[player]` | Gallery of Boss, Alpha, and Lucky pals. |
| `/collection` | `[player]` | 306-pal completion, missing species, and rare variants. |
| `/dex` | `<pal>` | Palworld 1.0 mechanics, learnset, work, ownership, and icon. Sections switch with a select menu. |
| `/workers` | `<job> [player]` | Rank worker pals for a base job. |
| `/team` | `<purpose> [player]` | Recommend a combat party or base-role roster. |
| `/progress` | `[player]` | Lifetime captures, unique captures, and Paldeck unlocks. |
| `/goal` | `add \| list \| remove` | Restart-safe collection goals that notify you on completion. |

## Breeding

| Command | Arguments | What it does |
|---|---|---|
| `/breed` | `<child> [player]` | Rank parent pairs for a target child by what is currently owned. |
| `/breedpath` | `<target> [scope] [player]` | Shortest breeding chain from an owned roster to a target. |

## History

| Command | Arguments | What it does |
|---|---|---|
| `/history` | `[filter]` | Paginated feed of joins, leaves, backups, and panel events. |
| `/trends` | `[window]` | Level, playtime, and roster movement over time. |
| `/records` | | Current server records for players, pals, and guilds. |

## AI assistant

| Command | Arguments | What it does |
|---|---|---|
| `/ask` | `<question> [private]` | Read-only Palworld assistant using live-server tools, pinned 1.0 knowledge, and cited web search. See [Ask assistant](/bot/ask-assistant/). |

## Admin

These need the admin role and act on the game server.

| Command | Arguments | What it does |
|---|---|---|
| `/backup` **ADMIN** | | Trigger a world backup now. |
| `/backups` **ADMIN** | | Recent backups plus the schedule. |
| `/announce` **ADMIN** | `<message>` | Broadcast an in-game message. |
| `/diagnostics` **ADMIN** | | Cache, knowledge, history, AI, and automation status. |
| `/profileadmin` **ADMIN** | `assign \| clear` | Manage other members' player links. |

:::caution
The admin commands run real actions on the server. `/backup` writes a world backup, and `/announce` broadcasts to everyone in game. Keep `ADMIN_ROLE_ID` scoped to people you trust.
:::

## Notes on images

`/map`, `/pals`, and the pal-icon parts of other commands render images on the panel host. If map tiles or pal icons were never fetched on that host, the bot falls back to text-only replies. See the panel's map tiles and icons setup for how those assets are downloaded.
