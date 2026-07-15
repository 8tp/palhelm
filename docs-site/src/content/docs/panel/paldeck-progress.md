---
title: Paldeck progression
description: Save-observed server and per-player capture progression against the pinned Palworld 1.0 catalog.
sidebar:
  order: 4
---

The **Save-observed Paldeck** compares authoritative player `RecordData` maps with Palhelm's pinned
Palworld 1.0 species catalog. Choose **Server union** or an individual player. Search the catalog or
filter to captured, unseen, or unavailable observations, then open matching owned instances in the
[Pal explorer](/panel/pal-explorer/).

Palhelm keeps three kinds of number separate:

- **Pinned species progress** counts only known catalog entries with authoritative per-species
  observations. A percentage appears only when capture or unlock coverage is complete and
  untruncated.
- **Aggregate captures** and the **save unique counter** come directly from player saves. A unique
  counter may include an ID outside the pinned catalog, so it is not used as the catalog percentage.
- **Unavailable** is not zero. A player without a decoded map remains unavailable. A partial server
  union cannot prove that a zero-count species is unseen by the whole server, so the unseen filter
  is disabled until every known player has complete, untruncated capture coverage.

The catalog response includes known species plus bounded unknown IDs actually observed in saves.
Boss-prefixed keys are normalized to their base species before aggregation. This is capture and
unlock state observed at the latest successful parse, not owned-Pal inference and not a lifetime
event timeline.

This screen reads viewer-safe `GET /api/v1/paldeck` and
`GET /api/v1/players/{uid}/paldeck`. Player selection is stored in the `player` URL query so a guild
member can link directly to that view.
