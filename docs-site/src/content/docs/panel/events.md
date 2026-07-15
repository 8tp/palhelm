---
title: Events and audit
description: The combined log of player activity, server operations, and panel changes.
sidebar:
  order: 7
---

This page covers the events and audit screen: the combined log of player activity, server operations, and changes made through the panel.

The events screen is one timeline for three kinds of history: who joined and left, what the server did, and what operators changed through the panel. Every row has a time, a type, the event message, and the actor who caused it when one is known.

The lane cards split that timeline into useful operational views:

- **Player activity** contains joins and leaves.
- **Operations & audit** contains backups, panel actions, and configuration changes.
- **Health incidents** contains system health transitions.

Select **All events** to return to the combined timeline. Each card's count is calculated from the same newest-event response; it is not a lifetime total.

## Event types

After selecting a lane, the exact-kind menu narrows that lane by type:

- **Joins** and **Leaves.** Players connecting and disconnecting.
- **Backups.** Snapshots taken, scheduled or manual.
- **System.** Health transitions such as REST reachability and save-format coverage changes.
- **Panel audit.** Actions taken through the panel, for example a kick, a ban, or a console command. The actor column shows which panel user did it.
- **Configuration.** Changes written to the Compose file through the [configuration editor](/panel/configuration/).

## Searching and paging

Search filters the selected lane and exact kind by message text. The screen fetches one bounded corpus containing at most the newest 500 events, calculates every lane count from that corpus, and pages matches 25 at a time. Use Previous and Next to move between pages. Counts are therefore coverage-qualified newest-window counts, not all-time totals.

The log refreshes on its own about every 30 seconds, and new events also arrive live over the event stream.

:::note
The console and the moderation actions on the [players](/panel/players/) screen are audit-logged here as panel events. The events screen is the record of what was done through the panel and by whom.
:::

## Data sources

This screen reads one newest-500 corpus from `GET /api/v1/events`, applies its lane, exact-kind, and search filters in the browser, and receives live updates over `GET /api/v1/events/stream`.
