---
title: Live map
description: The world map, player and base markers, tile layers, and why tiles are not shipped.
sidebar:
  order: 3
---

This page covers the live map: what it plots, the tile layers, the coordinate readout, and how to install the map tiles.

The map plots player and base positions on the Palworld 1.0 world. Live player positions come from the game REST API, so online markers stay current on their own. Base positions come from the parsed save data and update when save sync runs. A hint in the card header shows when the last sync happened.

## Using the map

Drag to pan. Scroll or use the plus and minus buttons to zoom. The panel picks the right tile resolution for the current zoom.

Two marker layers can be toggled on or off:

- **Players.** Online players that have a known position. Each marker shows the player name.
- **Bases.** Guild bases from the save. Each marker shows the guild name.

A coordinate readout in the corner follows the cursor. It always reads in Palworld's own in-game display coordinates, independent of the tile imagery.

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

The map reads `GET /api/v1/map/dataset` to learn which tile layers exist, then loads the tile images from the data volume. Markers come from `GET /api/v1/players` and `GET /api/v1/guilds`. The last sync time comes from `GET /api/v1/server/health`.
