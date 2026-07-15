---
title: Diagnostics
description: Read-only evidence for panel pollers, save parsing, Game Data, and backups.
sidebar:
  order: 8
---

The Diagnostics screen collects the panel's existing safe operational facts into one read-only
view. It shows REST and RCON reachability, save discovery, parse freshness and duration, bounded
format coverage, Game Data capability/freshness/latency, exact base-worker link coverage, retry
state, and backup schedule/freshness.

The page is safe for both admin and viewer sessions. It never displays raw Game Data actors,
upstream response bodies, credentials, filesystem paths, or SQLite internals. Where the current API
cannot provide a fact—such as filesystem headroom, database schema state, or a timestamp for the
latest metrics sample—the UI says it is unavailable instead of inventing a value.

Refreshing this route only performs authenticated `GET` requests. It does not trigger a save parse,
backup, configuration apply, announcement, shutdown, or game-server restart.
