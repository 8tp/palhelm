---
title: Guild details
description: Current members, bases, associated Pals, and bounded panel-observed guild activity.
sidebar:
  order: 5
---

The **Guilds** route lists the real player guilds from the latest parsed save. Open one for a
dedicated view of its current member roster, bases, associated Pals, and rolling 30-day activity.

The save records a group for more than just player guilds — a solo player's automatic organization
and other non-guild groups also appear in the raw data with no base placed and no confirmed player
member. The list leaves those out and shows only guilds that have both at least one base built and at
least one confirmed player. Membership evidence counts from either direction: a group-roster member
that resolves to a known player, or a known player whose own save record points back at the guild.
Real 1.0 saves carry base-owning guilds with an empty group roster whose players still reference
them, so the back-reference matters. Guilds you open by link still load in full even when they are
left out of the list, so a player who belongs to one of those groups can still reach it.

- Member names link to player detail and their save-observed Paldeck progression.
- Bases show the player's chosen name, or a `Base N` fallback when the base was never renamed, and
  their coordinates link to the exact location on the authenticated live map. A base whose location
  could not be decoded reads **Unavailable** rather than a made-up point.
- Pals are included only when they join an exact guild base or have a resolved current-member owner.
  Owner evidence stays qualified, and matching species link to the filtered Pal explorer. Each Pal
  shows its condenser star rating when the save recorded one; a Pal with no rank shows no stars
  rather than an implied zero.
- Activity is derived from sessions observed by this Palhelm installation and attributed to the
  guild's current member roster. It does not reconstruct historical guild transfers.

The detail endpoint returns at most 500 associated Pals and says when that bounded result was
truncated. Missing base locations and progression counters remain unavailable instead of being
rendered as zero.

The list reads `GET /api/v1/guilds`; detail reads viewer-safe
`GET /api/v1/guilds/{id}`. The path uses the normalized save guild ID.
