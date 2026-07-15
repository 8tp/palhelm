---
title: Live map
description: The world map, player and base markers, tile layers, and why tiles are not shipped.
sidebar:
  order: 3
---

This page covers the live map: what it plots, the tile layers, the coordinate readout, and how to install the map tiles.

The map plots player and base positions on the Palworld 1.0 world. When the optional
game-data poller is enabled and has a fresh snapshot, player markers use its sanitized live
coordinates. Otherwise they fall back to the normal REST player list. Base positions remain
save-derived and update when save sync runs. The card header states which source is active and
shows a stale/capability badge rather than silently presenting an old live snapshot as current.

The normal REST player list remains the roster authority. A non-truncated, ready game-data
snapshot can update a coordinate only when one active actor exactly and uniquely matches that
known player name. Partial data never hides known players, and extra, stale, inactive, or
ambiguous actors never become map markers.

## Using the map

Drag to pan. Scroll over the map or use its SVG zoom controls to zoom; the fit control returns to the whole active layer. Wheel and trackpad input is contained by the map while the pointer is over it, so zooming does not scroll or zoom the surrounding page. The panel picks the right tile resolution for the current zoom.

The action bar searches the currently known online players and save-derived guild bases. Selecting a result focuses it, enables its marker layer, and leaves it selected for the **Focus selected** action. Panel login names are not game identities, so Palhelm does not guess which player is "you". Use an explicitly selected online player instead.

**Fit online** frames every positioned online player covered by the active tile layer. **Fit bases** does the same for bases. Counts on those buttons are layer-qualified: a Palpagos marker is not included while viewing a World Tree layer that does not cover it.

Tap or click the map to pin its Palworld display coordinate. The coordinate control and **Copy coordinate link** copy a URL containing only `x`, `y`, and the active tile-layer id. Opening that URL restores the layer, pin, and focused position. Shared links never include player names, player IDs, guild names, or live actor data, and the recipient must still authenticate to view the map.

On narrow screens, the search and focus actions wrap into touch-sized controls, map-layer chips scroll horizontally, and zoom controls switch to a bottom row. A tap pins coordinates while a drag continues to pan.

Marker layers can be toggled on or off:

- **Players.** Online players that have a known position. Each marker shows the player name.
- **Bases.** Guild bases from the save. Each marker shows the guild name.
- **Workers.** Pals working at a base, shown only when the optional game-data poller is enabled and has a fresh snapshot. This dense layer starts off and can be enabled from the map. These are exact save-linked workers, so an actor becomes a worker marker only when it uniquely matches a known save instance. When game data is off, unauthorized, or stale, the map shows a badge instead of plotting workers.

Players, bases, workers, and Palboxes have separate SVG marker shapes as well as labels, so marker identity does not depend on color alone.

A **Live base health** panel beside the map summarizes those same exact-linked workers by base, with their current activity. It reads "No exact-linked live base workers are currently loaded" rather than guessing when the data is unavailable.

A coordinate readout in the corner follows the cursor until a point is pinned. It always reads in Palworld's own in-game display coordinates, independent of the tile imagery.

When the tile dataset reports more than one layer, a second row of toggles lets you switch the base imagery, for example between the Palpagos islands and the World Tree.

:::note
Markers are hidden on a layer that does not cover them. A player on Palpagos is not drawn while you view the World Tree layer, rather than being placed in the wrong spot.
:::

## Installing map tiles

The map needs terrain tiles. These tiles are derived from the game's own assets. They are copyrighted by Pocketpair, so Palhelm does not ship them. Until you install them, the map shows a "Map tiles not installed" panel with the command to run.

Fetch the tiles once with the script into your data volume:

```sh
scripts/fetch-map-tiles.sh ./palhelm-data/map-tiles
```

The fetch pulls a versioned tile dataset into the data volume. See [Map tiles and icons](/getting-started/map-tiles-and-icons/) for what the script downloads and where the files land.

:::caution
If the installed tiles are an older dataset, the map shows a "MAP DATA: PRE-1.0" badge over the view until you refresh them to the 1.0 dataset. The badge is a reminder that the terrain imagery is out of date, not an error.
:::

## Data sources

The map reads `GET /api/v1/map/dataset` to learn which tile layers exist, then loads the tile
images from the data volume. It reads the sanitized `GET /api/v1/world/snapshot` for optional
live player positions, with `GET /api/v1/players` as the fallback, and reads bases from
`GET /api/v1/guilds`. The save-sync time comes from `GET /api/v1/server/health`.
