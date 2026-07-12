---
title: Console
description: The RCON session, command history, saved commands, and the honest limits of vanilla RCON.
sidebar:
  order: 4
---

This page covers the console: a real RCON session with the game server, its history, saved commands, and the limits of vanilla RCON that the panel is honest about.

The console runs a real RCON session against the game server. Every command you send is audit-logged and appears in the [events log](/panel/events/). The connection status pill in the header shows whether RCON is connected.

## Sending commands

Type a command in the input and press Enter, or select Send. The output appears in the log above. Press the up arrow to cycle back through your command history, and the down arrow to move forward again.

A viewer sees the console read-only. The input is disabled and no commands can be sent.

## Saved commands

Save a command you run often. It appears in the saved commands list on the side for one-click reuse. An admin can add a new saved command with a name and a command string, run it, or delete it. Deleting a saved command removes the shortcut only. It does not run or undo anything on the server.

The command palette can insert a saved command into the input. It never runs it for you. RCON commands can be destructive, so you always review the command before sending. Open the palette with Cmd+K, or Ctrl+K on Windows and Linux.

## Command reference

The console includes a short inline reference for common vanilla RCON commands, for example `ShowPlayers`, `Broadcast`, `KickPlayer`, and `Shutdown`.

:::caution
Vanilla RCON is limited. There is no whisper or private message, and `Broadcast` mangles spaces: a message with spaces can arrive changed. In the reference, spaces are shown as underscores for this reason. Because of these limits, Palhelm prefers the game REST API for moderation actions such as kick and ban, and says so in the UI. Use the [players](/panel/players/) screen for moderation when you can.
:::

## Data sources

The console reads `GET /api/v1/console/log` for history and `GET /api/v1/console/saved` for saved commands. Commands run through `POST /api/v1/console/exec`. Saved commands are managed with `POST` and `DELETE /api/v1/console/saved`. Connection status comes from `GET /api/v1/server/health`.
