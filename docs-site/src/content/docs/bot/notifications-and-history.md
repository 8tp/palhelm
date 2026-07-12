---
title: Notifications and history
description: How the bot posts server notifications and activity, tracks milestones and a weekly digest, and stores history under BOT_DATA_DIR with drift handling.
sidebar:
  order: 5
---

This page covers how the bot posts notifications and activity into Discord, how milestones and the weekly digest work, and how it stores history under `BOT_DATA_DIR`, including how it handles save format drift. The settings mentioned here are documented in full on the [Configuration](/bot/configuration/) page.

## The shared snapshot boundary

One polling boundary drives everything. Every roughly five minutes the bot takes a single atomic snapshot of the world from the panel, and every command reads from that snapshot. Commands never poll the panel on their own. This keeps load predictable and keeps all readers consistent.

The first successful snapshot is a silent baseline. Nothing is announced from it. Milestone claims are always observations made since tracking began, not claims about the whole history of the server.

## Server notifications

Server notifications post to the channel in `NOTIFY_CHANNEL_ID`. They come from the panel's live event stream. Which kinds post is controlled by `NOTIFY_EVENT_KINDS`, which defaults to `backup,system`. The available kinds are `backup`, `system`, `join`, `leave`, `panel`, and `config`.

Typical notifications include backup completion notices and restart or shutdown countdowns.

:::note
TRANSCRIPT. Illustrative example with fictional content.
:::

```
/status
palworld · v1.0.0.100427 · 🟢 online
2/16 players · day 143 · 59 FPS · up 6d 4h

🟢 Kestrel came online
Backup complete. world_2026-07-12_0400.tar.gz · 412 MB · scheduled
```

## Activity feed

The join and leave feed can post to its own channel by setting `ACTIVITY_CHANNEL_ID`, so members can mute the chatty feed separately from the main notifications. Leave it blank to disable the feed.

Activity posts read like this:

:::note
TRANSCRIPT. Illustrative example with fictional content.
:::

```
🟢 mika_o came online
🔴 VossR went offline · played 2h 14m
```

Session durations are seeded from the panel's own session rows, so a bot restart does not lose an in-progress session. The bot's Discord presence reflects the server, for example "Playing Palworld · 3/16 online · up 2d 4h", and switches to Do Not Disturb when the server is unreachable.

### Burst coalescing

When several players join or leave close together, the feed groups the activity rather than posting a separate line for every event, so a busy moment does not flood the channel.

## Milestones

Milestones are enabled by default with `MILESTONES_ENABLED`. They post conservative, snapshot-inferred achievements, such as a first Alpha capture or a new level record. You can send them to their own channel with `MILESTONES_CHANNEL_ID`, otherwise they fall back to the notify channel.

Milestone posts use a locally rendered card when possible: the panel's cached Steam avatar for the attributed player, a cached Paldeck icon for pal milestones, and a level, playtime, or record badge otherwise. Missing or private avatars and missing pal icons fall back to an initials portrait or a badge, and if rendering fails the bot keeps the normal text embed. Observations that have no owner are not announced.

## Weekly digest

The weekly digest is opt-in. Enable it with `WEEKLY_DIGEST_ENABLED`. It posts on the local-time weekday in `WEEKLY_DIGEST_WEEKDAY` (0 is Sunday) at the hour in `WEEKLY_DIGEST_HOUR`. Times use the host's local timezone.

## History storage under BOT_DATA_DIR

Observation state is written restart-safely beneath `BOT_DATA_DIR`, which defaults to `data`. This is where snapshots, milestone history, goals, and digest state live. Pal goals are stored atomically and complete only when a new matching instance is observed after the goal was created.

Because this data is on disk, the bot survives restarts without losing history or in-progress goals.

## Save format drift handling

When the panel reports save format drift, for example while its parser catches up to a new Palworld version, the bot suppresses inferred milestones so it does not post claims from data it cannot trust. You can also mute the drift notices themselves with `NOTIFY_SUPPRESS_DRIFT`.

Guarded history tracking during drift is opt-in through `HISTORY_ALLOW_FORMAT_DRIFT`. Even when enabled, it only proceeds after two consecutive structurally consistent snapshots and rejects empty or collapsed results. The confirmation candidate is held in memory, so a bot restart requires two fresh consistent snapshots again. Keep it off unless an operator has inspected the data.

## Health alerts

Optional health alerts are enabled with `HEALTH_ALERTS_ENABLED`. They watch for sustained low FPS and stale-save or backup state using fresh-snapshot hysteresis, and they post a recovery notice when conditions return to normal.

:::caution
Health alerts are notification only. They never remediate, restart, or change anything on the server. They only tell you.
:::
