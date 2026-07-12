---
title: Dashboard
description: The overview screen, the server status strip, quick actions, and how live updates work.
sidebar:
  order: 1
---

This page covers the dashboard, the status strip at the top of every panel screen, the admin quick actions, and how the panel keeps its numbers current.

The dashboard is the first screen after login. It answers one question fast: is the server healthy right now, and what just happened. It shows history, not only the current number, so a dip is visible in context.

## What the dashboard shows

The top row holds four stat tiles:

- **Server FPS**, with the current frame time in milliseconds under it.
- **Players online**, out of the server maximum, with a count of players seen in the last 24 hours.
- **Base camps**, with the number of guilds that hold at least one base.
- **Last backup**, how long ago it ran, how many snapshots are kept, and their total size.

Below the tiles are the charts and tables:

- **Server performance.** A frame-rate chart with a smaller frame-time chart under it. Toggle the window between 60 minutes and 24 hours. A notable dip is annotated, for example a drop that lines up with a world save.
- **Server.** Name, game version, panel version, world id, and the live status of RCON, the REST API, and save sync.
- **Players online.** A 24 hour history of the connected player count.
- **Recent events.** The five most recent events, with a link to the full [events log](/panel/events/).

## The status strip

A status strip sits at the top of every panel screen. It stays visible as you move between pages. It shows the server state, a short FPS sparkline with the running average, the player count, the in-game day, and the uptime.

For an admin, the strip also carries three quick actions:

- **Broadcast.** Send a message to every player in game.
- **Save world.** Trigger an immediate world save on the running server.
- **Shut down.** Schedule a graceful shutdown. You set a wait time and a message, and can send staged warnings at 10, 5, and 1 minute, then 30 and 10 seconds. A viewer does not see these actions.

:::caution
The panel can stop the server gracefully, but it cannot start it again. Any restart depends on your own host supervisor or a container restart policy. Configure and verify that separately.
:::

:::note
Vanilla RCON mangles spaces in `Broadcast`. A broadcast message with spaces can arrive changed. This is a limit of the game server, not the panel. See the [console](/panel/console/) page for the details.
:::

## The command palette

Press Cmd+K, or Ctrl+K on Windows and Linux, to open the command palette from any screen. It searches players, jumps between pages, and inserts saved RCON commands. For an admin it also lists kick, ban, unban, and broadcast actions. A viewer never sees those destructive entries.

The palette never runs a destructive action on its own. A player action opens the same confirm dialog you would use on the [players](/panel/players/) screen. A saved command is inserted into the console input for you to review before sending.

## How live updates work

The panel uses a Server-Sent Events stream for live metrics, player changes, and new events. When the stream is not available, it falls back to periodic polling. Metrics refresh about every 5 seconds. You do not need to reload the page.

## Data sources

The dashboard reads `GET /api/v1/server`, `GET /api/v1/server/health`, `GET /api/v1/metrics/current`, `GET /api/v1/metrics/history`, `GET /api/v1/guilds`, `GET /api/v1/backups`, `GET /api/v1/players`, and `GET /api/v1/events`. Metrics and events also arrive over `GET /api/v1/events/stream`.
