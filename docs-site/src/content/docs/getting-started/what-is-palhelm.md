---
title: What is Palhelm
description: The parts of Palhelm, what it does for a Palworld server, and what it does not do.
sidebar:
  order: 1
---

This page covers what Palhelm is, the two parts you can run, and the limits it is honest about.

Palhelm is a self-hosted web admin panel for Palworld dedicated servers. It ships as one Docker image that runs one process with no external database. You run it next to the game server you already have, and you manage the server from a web browser on your own network.

Palhelm targets Palworld 1.0. It talks to the server over three channels: the official REST API, RCON, and the on-disk save files. The save reader is a pure-Go parser for the 1.0 Oodle-compressed `Level.sav` format.

## The two parts

Palhelm has two parts. You can run the panel alone, or add the bot later.

- **The panel** is the web app in the Docker image. It gives you a live dashboard, a players list, a console for RCON, a live map, backups with restore, and a config editor. This is the part most people run.
- **The Discord bot** is a separate, optional program. It reads from the panel through the read-only Integration API and posts activity to a Discord server. See the [Bot](/bot/setup/) section for setup. The bot never needs your admin login; it uses a scoped, redacted API key.

## What it does

- Shows server FPS and frame-time history, players-online history, and the health of each data channel.
- Merges online and offline players from the live API and the save files, with real Paldeck names and per-player Pals.
- Runs a real RCON session with command history and saved commands.
- Draws a live map of your world with player and base markers, and, when the optional live game data is enabled, real-time player positions and a base-workers layer.
- Takes scheduled and manual backups, and restores a snapshot after a dry-run diff and a typed confirmation.
- Edits your Compose file's `environment:` block for server settings, then shows you the exact host command to apply the change.
- Offers an admin login and an optional read-only viewer login.

## What it does not do

Palhelm is deliberately honest about its edges.

- **It does not restart your server.** It can run a graceful shutdown countdown, but it cannot start a stopped server. A separate host supervisor or a container restart policy must do that.
- **It does not enforce a whitelist.** Player notes are annotation data only. They do not control who may join.
- **It does not work around vanilla RCON limits.** Vanilla RCON has no whisper, and `Broadcast` mangles spaces. Palhelm prefers the REST API for moderation and says so in the UI.
- **It does not apply Docker changes for you.** One-click apply is intentionally disabled, because a container cannot safely preserve arbitrary host project paths. Palhelm prints the command; you run it on the host.
- **It does not degrade silently on a game update.** If a future patch drifts the save format, the affected feature shows a "format drift" badge instead of showing wrong data.

## Next steps

- [Install Palhelm](/getting-started/install/) with Docker Compose.
- [Log in the first time](/getting-started/first-login/) and set up the viewer role.
- [Fetch map tiles and Pal icons](/getting-started/map-tiles-and-icons/).
