---
title: Pal explorer
description: Search and filter every save-derived Pal on the server.
sidebar:
  order: 3
---

The Pal explorer searches the server-wide save roster without opening players one at a time.
Search by Pal name, internal species identifier, or known owner name. Filters narrow results by
party, Palbox, or base placement; owner evidence; level range; and Standard, Alpha, Lucky, or Boss
specimen.

Each result shows the resolved species name and icon, level, placement, the best available owner
evidence, and — when the save recorded one — the Pal's condenser star rating. Expand the info
control to see individual HP, gender, condenser stars, talents, passive and equipped skill
identifiers, plus the shared species work-suitability badges. Every work badge uses its own SVG
symbol and shows the numeric level, such as **Mining Lv 3**. A Pal with no recorded condenser rank
shows an empty four-star row in the expanded detail — an absent rank in the save means never condensed (the game only writes the rank once a Pal has been condensed). **Unavailable** appears only for data parsed before rank decoding shipped.

Owner evidence is deliberately qualified. A current personal container is stronger than the owner
stored in the save, while “last observed” is historical attribution rather than proof of current
possession. Unresolved Pals remain unresolved instead of being assigned by guesswork. Boss-prefixed
species are presented with their normal Pal name and a Boss marker.

Results use keyset pagination. The browser loads 48 at a time and stops at 480 visible matches;
narrow the filters to inspect a larger roster. Filtering happens in SQLite before pagination, so
the page does not download the complete save roster.

## Filtered links

The current filters are stored in the URL, so a record, history entry, guild page, or another
Palhelm integration can link directly to the matching roster. Supported query fields are `q`,
`ownerSource`, `placement`, `specimen`, `minLevel`, and `maxLevel`. The page validates every value
against the same bounded API contract and starts pagination from the beginning; opaque pagination
cursors are never put into shared links.

For example, `/pals?q=Mammorest&specimen=boss` opens the known Boss Mammorest results. The link is a
view of the latest parsed save when it is opened, not a frozen historical snapshot and not proof of
lifetime ownership.

This screen reads the authenticated, viewer-safe `GET /api/v1/pals` endpoint. It never receives raw
save JSON, Steam ids, account names, or platform identifiers. It is separate from the public
Integration API used by bots.
