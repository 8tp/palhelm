---
title: Events and audit
description: The combined log of player activity, server operations, and panel changes.
sidebar:
  order: 7
---

This page covers the events and audit screen: the combined log of player activity, server operations, and changes made through the panel.

The events screen is one timeline for three kinds of history: who joined and left, what the server did, and what operators changed through the panel. Every row has a time, a type, the event message, and the actor who caused it when one is known.

## Event types

Filter the log by type:

- **Joins** and **Leaves.** Players connecting and disconnecting.
- **Backups.** Snapshots taken, scheduled or manual.
- **System.** Server operations such as saves and shutdowns.
- **Panel audit.** Actions taken through the panel, for example a kick, a ban, or a console command. The actor column shows which panel user did it.
- **Configuration.** Changes written to the Compose file through the [configuration editor](/panel/configuration/).

## Searching and paging

Search filters the visible rows by message text. The screen scans the newest 500 events and pages the matches 25 at a time. Use Previous and Next to move between pages. The count line shows how many events match and how many were scanned.

The log refreshes on its own about every 30 seconds, and new events also arrive live over the event stream.

:::note
The console and the moderation actions on the [players](/panel/players/) screen are audit-logged here as panel events. The events screen is the record of what was done through the panel and by whom.
:::

## Data sources

This screen reads `GET /api/v1/events`, with an optional `kind` filter, and receives live updates over `GET /api/v1/events/stream`.
