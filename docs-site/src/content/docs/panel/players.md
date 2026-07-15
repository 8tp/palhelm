---
title: Players
description: The players list, guilds, player notes, bans, and the per-player Pal box.
sidebar:
  order: 2
---

This page covers the players screen: the merged online and offline list, moderation actions, guilds, player notes, bans, and the Pal box.

The players screen merges two sources. Live players come from the game REST API. Offline players come from the parsed save data. A player appears here once they have joined the server at least once. The header shows how many are online and how many are known from save data.

The screen has four tabs: Players, Guilds, Player notes, and Bans.

## Players tab

The table lists each player with an avatar, name, Steam id, status, level, guild, ping, and last seen time. Search by name or Steam id. Filter by all, online now, or offline. A hint shows when the save data was last synced.

Steam avatars are proxied through the panel on the same origin, so the browser never fetches images from an outside host. The panel ships a built-in Paldeck name roster covering the 1.0 species and variants, so Pals show their real names instead of raw internal ids.

Select a row to open the detail panel on the right. It shows the player level, guild, in-game position, ping, first seen date, and playtime. Under that is the Pal section.

### Moderation actions

An admin sees an actions menu on each row and in the detail panel:

- **Message.** The vanilla server has no private messages, so this broadcasts to all players with the name as a prefix, for example `@Kestrel: ...`.
- **Show on map.** Opens the [live map](/panel/live-map/) focused on that player. Available only when the player has a known position.
- **Kick.** Disconnects an online player. They can rejoin immediately. You can attach a message.
- **Ban.** Bans the player's Steam id until you unban them. If they are online, they are disconnected now.
- **Unban.** Lifts the ban so the player can rejoin.

Each action opens a confirm dialog first. Kick and ban let you type an optional message shown to the player.

:::note
A viewer has read-only access. Viewers see the list and the detail panel, but none of the moderation actions.
:::

### The Pal box

The detail panel shows the player's active party first, up to five Pals from the save. Each entry has an info button that expands to show level, individual HP, gender, talents, passive skill ids, and equipped skill ids, plus alpha and lucky markers. It also joins the Pal's CharacterID to the bundled, version-pinned species catalogue and shows every available work suitability as a labeled SVG badge with its numeric level, such as **Handiwork Lv 4**. Save observations and species metadata remain labeled separately.

Select "show all" to open the Pal box dialog. It recreates the in-game storage layout:

- A **Party** page with five slots.
- Numbered **Box** pages with 30 slots each. Empty slots render as empty cells, just like in game.
- A **Base and expeditions** page for Pals the save placed outside the party and box.

:::note
Pal data comes from the last save parse, not live memory. It updates when save sync runs, not the instant a Pal changes in game. Some older parses may not carry party or box placement. When that happens the panel falls back to showing Pals in level order.
:::

## Guilds tab

Lists each guild with its member count, base count, and the roster of known members. Guild data is parsed from the save file.

## Player notes tab

Player notes are local Steam id annotations. You can label a Steam id with a name inside the panel. This is stored by the panel only.

:::caution
Player notes are not a whitelist. This list does not control who may join the server. The panel does not enforce an allow-list. Authoritative allow-list enforcement is deferred until a supported vanilla or verified integration exists.
:::

## Bans tab

Lists the players you have banned so a ban can be reviewed or lifted. Each row has an unban action for admins.

## Data sources

This screen reads `GET /api/v1/players`, `GET /api/v1/players/{uid}` for detail, `GET /api/v1/guilds`, and `GET/PUT /api/v1/whitelist` for player notes. Moderation uses `POST /api/v1/players/{uid}/kick`, `/ban`, and `/unban`. Messaging uses `POST /api/v1/server/announce`.
