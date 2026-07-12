---
title: Project layout
description: A tour of the Palhelm repository. Where the backend, frontend, bot, docs, and support scripts live.
sidebar:
  order: 2
---

This page is a tour of the repository so you can find your way around. It covers the top
level and the parts of the backend, frontend, and bot you are most likely to touch.

## Top level

```text
palhelm/
  backend/     Go server: API, pollers, engines, save parser, storage
  frontend/    React and TypeScript web app, embedded into the binary
  docs/        source-of-truth docs: architecture, API, specs, roadmaps
  docs-site/   this documentation site (Astro and Starlight)
  website/     the public showcase site
  scripts/     support scripts, including the tile and icon fetchers
  examples/    example configuration
  assets/      shared assets
  Makefile     build and test targets
  Dockerfile   multi-stage build for the shipping image
  LICENSE      Apache-2.0
```

## Backend

The Go module is rooted at `backend/`. Entry points live under `cmd/`, and the real work
lives under `internal/`.

```text
backend/
  cmd/
    palhelm/        the main server: `palhelm serve`
    savdump/        parses a save file and prints JSON, for manual inspection
    paldeck-list/   a small Paldeck helper
  internal/
    server/         HTTP routing, handlers, auth, the OpenAPI spec
    poller/         the metrics, players, and save-sync pollers
    palworld/       clients for the official REST API and RCON
    sav/            the save parser: container, Oodle, GVAS, decoders
    store/          SQLite access and the migration runner
    store/migrations/  the ordered NNN_name.sql schema migrations
    backup/         the backup and restore engine
    config/         environment variable loading for the server
    gameconfig/     the Compose-file game settings editor
    paldeck/        Pal icon and Paldeck roster support
    steamavatar/    Steam avatar proxying
    webdist/        the embedded frontend build
```

The save parser is split into small files by concern: the container reader, the Oodle
wrapper, the GVAS reader, the property decoders, the character and group decoders, and
the world assembler. See the [save parser](/architecture/save-parser/) page for how it
works, and the [storage and migrations](/architecture/storage-and-migrations/) page for
the schema.

## Frontend

```text
frontend/
  src/
    routes/      one folder per screen
    components/  shared UI components
    app/         app shell, routing, shared state
    api/         the panel API client
    styles/      token CSS from the design system
    assets/      icons and images
  package.json   dependencies and scripts
```

The frontend uses plain token CSS, no Tailwind and no UI kit, so the design system CSS
ships as written. It is built by Vite and embedded into the Go binary at build time.

## Bot

```text
bot/
  src/
    commands/    slash command definitions
    discord/     Discord client and command registration
    palhelm/     the Integration API client
    ai/          the assistant and its tool loop
    knowledge/   the Pal knowledge data
    notify/      the notification and activity bridge
    history/     trends and history storage
    config.ts    environment configuration
    index.ts     entry point
  test/          the vitest suite
  .env.example   documented configuration template
  THIRD_PARTY-NOTICES.md
```

The bot is a separate Node project. It talks to Palhelm through the Integration API and,
for a few administrative features, the session API. See the [development](/contributing/development/)
page for how to run it.

## Docs and this site

`docs/` holds the source-of-truth documents that this site is written from:
`ARCHITECTURE.md`, `API.md`, the specs under `specs/`, and the roadmaps. This site,
under `docs-site/`, is an Astro and Starlight project that presents that material for
readers. When the two disagree, the repository is correct.
