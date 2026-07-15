---
title: Player activity
description: Bounded concurrency, peak hours, active players, and current-guild activity from panel-observed sessions.
sidebar:
  order: 4
---

The Activity screen summarizes connection sessions observed by this Palhelm installation. Choose a rolling **24 hour**, **7 day**, or **30 day** window.

It shows:

- Active players, split into **first observed in this window** and **previously observed** players.
- Peak concurrency and a bounded 24, 28, or 30-bucket concurrency timeline.
- The strongest peak-hour buckets, formatted in your browser's local time.
- Up to 25 players ranked by observed duration and session count.
- Up to 25 guilds ranked by attributed observed duration, session count, and active members.

Every session is clamped to the selected window. For example, a session beginning before the 7-day boundary contributes only the part after that boundary. Concurrency buckets contain average and peak observed players, not raw connection rows.

## Coverage and attribution

`trackingSince` records the earliest session available to this panel. If tracking began partway through the selected window, the screen says so. **First observed** means the player's first session in this panel's database falls inside the window; it does not prove that the player is new to Palworld or the server.

Guild history is not stored. Session activity is attributed using each player's **current save-derived guild**, and players without current guild evidence remain explicitly unattributed. Historical transfers therefore are not reconstructed or guessed.

The API caps player and guild rankings at 25 and returns only aggregate concurrency buckets. A defensive 100,000-interval analysis cap is also explicit if reached. The endpoint never returns raw join/leave rows, Steam ids, platform accounts, locations, or lifetime-history claims.

## Data source

This screen reads authenticated, viewer-safe `GET /api/v1/activity?window=24h|7d|30d`. It uses only the existing Palhelm `sessions` table and requires no save edit or database migration.
