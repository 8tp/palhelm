---
title: Settings and keys
description: Connection health, the theme, save sync, and Integration API key management.
sidebar:
  order: 8
---

This page covers the panel settings screen: connection health, authentication reference, the theme, save sync controls, and Integration API keys.

## Connections

The connections card shows the live status of each data channel: the game REST API, RCON, the mounted save volume, and the Compose file. It confirms at a glance that the panel can reach everything it needs.

## Authentication

The authentication card is a reference, not an editor. The admin password and the optional viewer password are set through environment variables when you run the container, and the card confirms what is configured without showing the values. The card also shows the login session duration, which is set through `PALHELM_SESSION_DAYS` (7 days by default) and now honored. The game server's admin password is never sent to the browser.

## Theme

The appearance card sets the theme to system, dark, or light. System follows your operating system preference. The choice takes effect immediately and is remembered in the browser.

## Save sync

The save sync card shows when the save was last parsed, how long the parse took, and how many guilds and players it found, along with a count of skipped properties. The sync interval is set through an environment variable and shown here for reference. An admin can select "Parse now" to force an immediate re-parse.

:::note
When a game update changes the save format, the panel does not show wrong data. It degrades the affected feature and shows a "format drift" badge on this card instead of breaking. The parser reads players, Pals, guilds, and bases, and skips sections the panel does not need, such as items, foliage, and dungeons.
:::

## Game Data API diagnostics

When the optional live game-data poller is enabled, this card shows the health of the shared actor snapshot the panel and Integration API read from. It reports the snapshot's freshness and state, how long the last upstream request took, the number of loaded actors, current and average FPS, how many base workers were exactly linked to save instances, any unresolved workers, the last poll result, and when the next attempt is scheduled. A line at the bottom breaks the workers down by activity: working, transporting, eating, sleeping, idle, incapacitated, and unknown.

The card reads aggregates and diagnostics only. It never lists actor identities, names, health, guilds, or locations. When the poller is off, unauthorized, or has no accepted snapshot, the state pill and freshness row say so instead of showing stale numbers as current.

## Integration API keys

The Integration API is a separate read-only, bearer-token API for bots, dashboards, and scripts. This card manages its keys. It is admin-only. A viewer never sees it, and the underlying API refuses viewer requests.

To create a key, select "New key" and give it a label, for example `discord-bot`. The label identifies the key in the list and is never sent to the bot. The plaintext key is shown exactly once, right after creation. Copy it then. The panel stores only a hash and can never show the key again. A key looks like this:

```
phk_a1b2c3d4_<43 more characters, shown once>
```

To stop a key from working, select "Revoke". Revocation takes effect on the very next request, with no restart. Revoked keys stay in the list for the audit trail and cannot be un-revoked. To replace a key, issue a new one. Up to 100 active keys are allowed.

:::caution
Treat these keys like passwords and serve the panel over TLS. The Integration API redacts sensitive fields by design, so a leaked key exposes far less than a session, but it still grants read access. For the full key lifecycle, redaction model, and endpoint reference, see the [Integration API](/integration-api/overview/) section.
:::

## Data sources

This screen reads `GET /api/v1/server`, `GET /api/v1/server/health`, `GET /api/v1/config`, and `GET /api/v1/world`. A forced parse uses `POST /api/v1/world/parse`. Keys are managed with `GET`, `POST`, and `DELETE /api/v1/integration-keys`.
