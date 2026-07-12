---
title: License and attributions
description: Apache-2.0, the unaffiliated fan-project disclaimer, and third-party attributions for data and assets Palhelm uses but does not bundle.
sidebar:
  order: 4
---

This page covers Palhelm's license, the unaffiliated fan-project disclaimer, and the
third-party data and assets Palhelm relies on. Several of these are fetched at run time
rather than shipped, for licensing reasons; those distinctions matter and are called out
below.

## Palhelm's license

Palhelm's own code is licensed under the Apache License, Version 2.0. The full text is in
the `LICENSE` file at the repository root.

## Unaffiliated fan project

Palhelm is an unaffiliated, fan-made tool. The Palworld name, game assets, and map imagery
belong to Pocketpair. Palhelm is not endorsed by or associated with Pocketpair, and it is
built to fit within their fan-work guidelines. Pal names and game concepts belong to
Pocketpair.

## Game-derived assets are not shipped

Two categories of art are derived from the game and are never bundled in the repository or
the Docker image. Palhelm fetches them at run time into the data volume, on the operator's
own machine:

- Map tiles. The live map uses tiles from a third-party provider. The provider's tile
  size, zoom range, bounds, and transform values are stored per dataset and preserved
  verbatim, because those values are what make the coordinate readout correct.
  `scripts/fetch-map-tiles.sh` downloads an operator-selected set of tiles into the data
  volume. Until tiles exist, the map screen shows a proper empty state.
- Pal icons. Pal preview icons are game-derived art. `scripts/fetch-pal-icons.sh`
  downloads them into the data volume. A missing icon falls back to an initials avatar, so
  nothing breaks if an operator chooses not to fetch them.

The map-tiles-and-icons page in getting-started covers these scripts in more detail.

## The Oodle decompressor is not redistributed

The 1.0 save format uses Oodle compression. The Oodle decompressor is proprietary and
cannot be redistributed, so Palhelm never commits it or bakes it into the image. It is
downloaded once at first save parse into the data volume and verified against a pinned
SHA-256 before use, or supplied by the operator. See the
[save parser](/architecture/save-parser/) page for the details.

## Third-party attributions

### PalCalc, MIT

The Discord bot's Pal knowledge cache is built from PalCalc, which is MIT licensed. PalCalc
supplies the canonical Palworld 1.0 roster and breeding matrix, along with expanded
mechanical fields such as stats, work suitability, and learnsets. The bot records the
source name, version, and license alongside the cached data. PalCalc's partner-skill field
is unpopulated in the pinned release, so the bot does not claim to have that data.

The bot also uses a pinned mechanical dataset that states MIT in its own README but ships
no standalone license file. Because of that gap, the bot deliberately excludes that
dataset's descriptions and artwork, keeps only the mechanical fields, and retains source
provenance.

### The CC BY-SA knowledge corpus

The bot's general Palworld knowledge summaries are adapted from The Palworld Wiki and are
licensed under Creative Commons Attribution-ShareAlike 4.0. The summaries are condensed and
reworded for the bot, and each stored section keeps its source page URL, revision id,
retrieval timestamp, source label, and license. Only this adapted corpus is offered under
CC BY-SA 4.0; Palhelm's own code stays under Apache-2.0. This corpus is runtime data and is
not included in Palhelm distributions. The full attributions, including the specific source
pages, are in `bot/THIRD_PARTY-NOTICES.md`.

### Map tile transforms

The map's coordinate transforms come from the tile provider's published dataset metadata
and are preserved verbatim. As noted above, the tiles themselves are fetched by the
operator and are not shipped.
