---
title: Setup
description: Create the Discord application, invite the bot with the right permissions, then register commands and start it.
sidebar:
  order: 1
---

This page covers creating the Discord application, the invite permissions the bot needs, why it uses no privileged intents, and how to register commands and start the process. For the full list of environment variables, see [Configuration](/bot/configuration/).

## Before you start

The bot talks to a running Palhelm panel. You need:

- A Palhelm panel you can reach over the network, and its admin password.
- An integration key from that panel (the `phk_` key, created in the panel and shown once). See [Keys and redaction](/integration-api/keys-and-redaction/).
- Node.js and npm on the machine that will run the bot.
- A Discord server where you can manage roles and channels.

## 1. Create the Discord application

1. Open the [Discord Developer Portal](https://discord.com/developers/applications) and create a new application.
2. On the **Bot** tab, choose **Reset Token** and copy the token. Treat it like a password. It goes in `DISCORD_TOKEN` and must never be committed.
3. On the **General Information** tab, copy the **Application ID**. This is `DISCORD_APPLICATION_ID`.

### No privileged intents

The bot only requests the **Guilds** gateway intent. Leave **Presence Intent**, **Server Members Intent**, and **Message Content Intent** switched off on the Bot tab. The bot never reads message content and never needs the member or presence privileged intents.

:::note
The Guilds intent is a standard, non-privileged intent. You do not need to request access to any privileged intent for the bot to work.
:::

## 2. Invite the bot

Invite the application with the `bot` and `applications.commands` scopes. The bot needs three channel permissions and nothing more:

- **Send Messages**
- **Embed Links**
- **Attach Files** (for rendered map and pal images)

Build the invite URL with your own application ID in place of the zeros:

```
https://discord.com/oauth2/authorize?client_id=000000000000000000&scope=bot%20applications.commands&permissions=51200
```

The permission integer `51200` is exactly Send Messages, Embed Links, and Attach Files. The bot does not ask for message management, kick, ban, or any moderation permission.

## 3. Collect the IDs you need

Enable **Developer Mode** in Discord (User Settings, Advanced) so you can copy IDs.

- Right-click your server, **Copy Server ID**. This is `DISCORD_GUILD_ID`.
- Right-click the channel for backup and server notifications, **Copy Channel ID**. This is `NOTIFY_CHANNEL_ID`.
- Right-click the role that should run admin commands, **Copy Role ID**. This is `ADMIN_ROLE_ID`.

All Discord IDs are long numbers. In this documentation they are shown as `000000000000000000`.

## 4. Configure the environment

Copy the example file and fill it in. Every variable is documented inline in the example and in full on the [Configuration](/bot/configuration/) page.

```sh
cp .env.example .env
```

At minimum you must set the Discord token, application ID, guild ID, notify channel ID, admin role ID, panel base URL, integration key, and admin password.

## 5. Register commands and start

Install dependencies, register the slash commands, then start the bot:

```sh
npm install
npm run register
npm start
```

Registration is guild-scoped, so the commands appear in your server immediately. Re-run `npm run register` whenever the set of commands changes.

For watch mode during development, use `npm run dev`.

:::tip
Keep the bot running with a process supervisor of your choice, such as a systemd unit, a container, or pm2. The bot re-logs in to the panel automatically when its session expires.
:::

## What to read next

- [Configuration](/bot/configuration/) for the complete environment variable reference.
- [Commands](/bot/commands/) for every slash command and its arguments.
- [Safety model](/bot/safety-model/) for how the two keys and role gating work.
