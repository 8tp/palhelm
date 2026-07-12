---
title: Safety model
description: The bot's two-key design, what is safe in a public channel, how admin actions are gated, and why it uses only the Guilds intent.
sidebar:
  order: 6
---

This page explains the bot's safety model: the two keys it uses and what each can do, why its read data is safe in a public channel, how admin actions are gated, and why it requests only the Guilds intent. The goal is that a public channel stays safe while server-changing actions stay tightly controlled.

## Two keys, two levels of access

The bot holds two credentials, and they are deliberately different.

- **The integration key (`phk_`)** is read-only. Reads such as `/status`, `/players`, `/pals`, `/map` data, `/guilds`, and `/metrics` go through the panel's [Integration API](/integration-api/overview/) with this bearer key.
- **The admin password** covers the parts the integration key cannot: triggering backups, the live event stream that powers notifications, in-game announcements, and fetching binary map tiles and pal icons. These use the panel's session API by logging in with the admin password, and the bot re-logs in automatically when its session expires.

Keep the admin password readable only by the account that runs the bot. See [Configuration](/bot/configuration/) for where each key is set.

## Redacted by design: safe in a public channel

The integration key surface is redacted by design. What it returns is safe to show in a public channel. It does not include Steam IDs, live player positions, ping, or ban and moderation state. See [Keys and redaction](/integration-api/keys-and-redaction/) for the full redaction model.

This is why the read commands are open to everyone: there is nothing sensitive to leak.

## Admin actions need two things

The five admin commands (`/backup`, `/backups`, `/announce`, `/diagnostics`, and `/profileadmin`) act on the game server. They are gated twice:

- The Discord member must hold the role in `ADMIN_ROLE_ID`.
- The action itself runs through the panel session API, which requires the admin password the bot holds.

So an admin command needs both a trusted Discord role and the panel's own authenticated session. Scope `ADMIN_ROLE_ID` to people you trust, because these commands broadcast to players and write backups.

## The AI assistant cannot change anything

The optional `/ask` assistant has only read-only tools. It has no session, admin, backup, announcement, shell, or mutation tool. It cannot act on the server. See [Ask assistant](/bot/ask-assistant/) for its full tool set and refusal scope.

## Only the Guilds intent

The bot requests only the **Guilds** gateway intent. It does not use any privileged intent: not Presence, not Server Members, not Message Content. It never reads message content. Its invite asks for just Send Messages, Embed Links, and Attach Files. See [Setup](/bot/setup/) for the exact invite permissions.

## Health watch never remediates

The optional health alerts are notification only. When the bot detects sustained low FPS or a stale save, it posts a warning and later a recovery notice. It never restarts, remediates, or changes anything on the server. Acting on an alert is always a human decision.
