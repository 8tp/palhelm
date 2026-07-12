---
title: Overview
description: How Palhelm fits together. One process, an embedded web app, and a single SQLite file.
sidebar:
  order: 1
---

This page covers the shape of Palhelm as a running system: what the parts are, how
they are packaged, and where data lives. The pages that follow go deeper on the
[data channels](/architecture/data-channels/), the [save parser](/architecture/save-parser/),
[storage and migrations](/architecture/storage-and-migrations/), and the
[security model](/architecture/security-model/).

## One process, one image

Palhelm ships as a single Docker image that runs a single Go binary. There is no
separate database server, no background worker fleet, and no Node runtime at run time.
The React web app is built ahead of time and embedded into the binary with `go:embed`,
so the same process serves the API and the web app on one port.

State lives in one SQLite file inside a mounted data volume. Palhelm uses the pure-Go
`modernc.org/sqlite` driver, so the binary builds with `CGO_ENABLED=0` and stays a
single static file. Backups, downloaded map tiles, downloaded Pal icons, and the
runtime-downloaded save decompressor also live in that data volume.

## Component diagram

```text
                          Trusted network (VPN, tailnet, or localhost)
                                          |
                                          v
  +--------------------------------------------------------------------+
  |                        palhelm  (one process)                      |
  |                                                                    |
  |   HTTP server on :8080                                             |
  |     /                       embedded React app (go:embed)          |
  |     /api/v1/*               session-cookie panel API               |
  |     /api/integration/v1/*   bearer-token, read-only API            |
  |     /healthz                healthcheck                            |
  |                                                                    |
  |   Pollers                        Engines                           |
  |     metrics   every 5s             backup   browse and restore     |
  |     players   every 15s            config   compose-file editor    |
  |     savesync  every 10m            actions  shutdown, save, kick   |
  |                                                                    |
  |   Storage:  one SQLite file in the data volume                     |
  +--------------------------------------------------------------------+
        |                    |                         |
        | REST 8212          | RCON 25575              | read-only
        v                    v                         v
  +-----------+        +-----------+           +-------------------+
  | Palworld  |        | Palworld  |           | Saved/ save files |
  | REST API  |        | RCON      |           | (mounted volume)  |
  +-----------+        +-----------+           +-------------------+
```

Palhelm talks to the Palworld dedicated server over three channels: the official
REST API, RCON, and the read-only save files. Each feature draws from one or more of
these. The [data channels](/architecture/data-channels/) page maps features to
channels in full.

## What runs on a timer

Three pollers run on their own intervals and write into SQLite:

- The metrics poller reads the REST metrics endpoint every 5 seconds and stores server
  frame rate, frame time, and player count.
- The players poller reads the REST players endpoint every 15 seconds and tracks join
  and leave sessions.
- The save-sync poller parses the world save file every 10 minutes and refreshes the
  players, pals, guilds, and bases tables. It can also run on demand.

All three intervals are configurable, through `PALHELM_METRICS_INTERVAL`,
`PALHELM_PLAYERS_INTERVAL`, and `PALHELM_SAVE_SYNC_INTERVAL`.

## What runs on request

Engines handle operator actions that do not fit a poller: scheduled and manual backups
with browse and guided restore, the config editor that writes to the Compose file's
environment block, and server actions like graceful shutdown, save, kick, ban, and
announce. The security-relevant details of these live in the
[security model](/architecture/security-model/) page.

## Two HTTP surfaces

The process exposes two API surfaces that never overlap:

- The panel API at `/api/v1/*` is authenticated with a session cookie and is what the
  embedded web app calls.
- The Integration API at `/api/integration/v1/*` is a separate, read-only, bearer-token
  API for bots, dashboards, and scripts. It is a distinct sub-router with its own
  middleware and its own redaction. A session cookie can never reach it, and a bearer
  token can never reach the panel API.

See the [security model](/architecture/security-model/) for how these two surfaces are
kept apart, and the [Integration API](/integration-api/overview/) group for the full
reference.

## Stack at a glance

| Layer | Choice |
|---|---|
| Backend | Go with `net/http` and the chi router |
| Storage | SQLite through `modernc.org/sqlite`, pure Go, no CGO |
| Web app | React and TypeScript built with Vite, plain token CSS |
| Charts | uPlot |
| Map | Custom pan and zoom tile renderer, no map library |
| Distribution | One Docker image, web app embedded with `go:embed` |
