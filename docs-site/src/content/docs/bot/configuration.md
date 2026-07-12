---
title: Configuration
description: The full environment variable reference for the Discord bot, with required and optional settings and their defaults.
sidebar:
  order: 2
---

This page is the complete environment variable reference for the bot. It lists every setting the bot reads, whether it is required, what it does, and a placeholder example. All example values here are fictional. Never commit a real `.env`.

The bot loads settings from a `.env` file next to the code. Copy `.env.example` to `.env` and fill it in. Discord IDs are shown as `000000000000000000` and are not real.

## Required settings

The bot refuses to start if any of these are missing.

| Variable | What it does | Placeholder example |
|---|---|---|
| `DISCORD_TOKEN` | Bot token from the Discord Developer Portal, Bot tab. Treat like a password. | `your-discord-bot-token` |
| `DISCORD_APPLICATION_ID` | Application ID from the General Information tab. | `000000000000000000` |
| `DISCORD_GUILD_ID` | Server ID where commands are registered and posted. | `000000000000000000` |
| `NOTIFY_CHANNEL_ID` | Channel for backup, restart, and server notifications. | `000000000000000000` |
| `ADMIN_ROLE_ID` | Discord role allowed to run admin commands. Scope it to trusted people. | `000000000000000000` |
| `PALHELM_BASE_URL` | Base URL of the Palhelm panel, no trailing slash. | `https://panel.example.com` |
| `PALHELM_INTEGRATION_KEY` | Read-only integration key from the panel. Shown once at creation. | `phk_a1b2c3d4_<43 more characters, shown once>` |
| `PALHELM_ADMIN_PASSWORD` | Panel admin password for the parts the read-only key cannot cover. | `your-panel-admin-password` |

:::caution
`PALHELM_ADMIN_PASSWORD` grants the bot the panel actions the read-only integration key cannot perform: triggering backups, the live event stream that powers notifications, in-game announcements, and fetching map tiles and pal icons. Keep the `.env` file readable only by the account that runs the bot. See the [Safety model](/bot/safety-model/) for how the two keys differ.
:::

## Channels and notifications

| Variable | Required | What it does | Default / example |
|---|---|---|---|
| `ACTIVITY_CHANNEL_ID` | Optional | Separate channel for the chatty join and leave feed, so members can mute it on its own. Blank disables the feed. | blank |
| `MILESTONES_CHANNEL_ID` | Optional | Dedicated channel for milestone posts. Falls back to `NOTIFY_CHANNEL_ID` when blank. | blank |
| `NOTIFY_EVENT_KINDS` | Optional | Comma-separated event kinds posted to the notify channel. Available: `backup`, `system`, `join`, `leave`, `panel`, `config`. | `backup,system` |
| `NOTIFY_SUPPRESS_DRIFT` | Optional | Set `true` to temporarily mute save format drift notices, for example while the panel parser catches up to a new Palworld version. | `false` |

## History and social features

| Variable | Required | What it does | Default / example |
|---|---|---|---|
| `BOT_DATA_DIR` | Optional | Directory for restart-safe snapshots, milestone history, goals, and digest state. Relative paths resolve from the bot directory. | `data` |
| `HISTORY_ALLOW_FORMAT_DRIFT` | Optional | Permit guarded history tracking while the panel reports save format drift. Only takes effect after two consecutive structurally consistent snapshots. Keep `false` unless an operator has inspected the data. | `false` |
| `MILESTONES_ENABLED` | Optional | Post conservative, snapshot-inferred milestones after a silent first baseline. | `true` |
| `WEEKLY_DIGEST_ENABLED` | Optional | Opt in to the weekly summary post. | `false` |
| `WEEKLY_DIGEST_WEEKDAY` | Optional | Local-time weekday for the digest, where 0 is Sunday and 6 is Saturday. | `0` |
| `WEEKLY_DIGEST_HOUR` | Optional | Local-time hour for the digest, 0 to 23. | `18` |
| `HEALTH_ALERTS_ENABLED` | Optional | Opt in to sustained low-FPS and stale-save alerts. Notification only. Never restarts anything. | `false` |

See [Notifications and history](/bot/notifications-and-history/) for how these features behave.

## Optional AI assistant

The `/ask` command is off until you set `OPENROUTER_API_KEY`. Web search is off until you also set `SEARXNG_URL`.

| Variable | Required | What it does | Default / example |
|---|---|---|---|
| `OPENROUTER_API_KEY` | Optional | OpenRouter key that enables `/ask`. When blank, `/ask` reports that it is unavailable. Use a provider that denies data collection and requires zero data retention. | blank |
| `OPENROUTER_MODEL` | Optional | Model used for `/ask`. | `deepseek/deepseek-v4-flash` |
| `AI_TIMEOUT_MS` | Optional | Deadline per model round trip. Tool-using questions can make several calls. Integer 5000 to 120000. | `60000` |
| `AI_DAILY_REQUEST_LIMIT` | Optional | Maximum `/ask` requests per day. Integer 1 to 10000. | `100` |
| `AI_COOLDOWN_SEC` | Optional | Per-user cooldown between `/ask` requests. Integer 0 to 3600. | `30` |
| `SEARXNG_URL` | Optional | Base URL of a self-hosted SearXNG instance for Palworld-scoped web lookups. Blank keeps `/ask` on server and pinned data only. | `https://searxng.example.com` |
| `WEB_SEARCH_TIMEOUT_MS` | Optional | Deadline per web search. Integer 1000 to 30000. | `8000` |
| `WEB_SEARCH_CACHE_TTL_SEC` | Optional | Lifetime of the successful-search cache. Zero disables caching. Integer 0 to 604800. | `21600` |

:::note
The SearXNG instance must enable the JSON response format. In its `settings.yml`, set `search.formats` to include both `html` and `json`. Without JSON output the bot cannot read search results.
:::

The AI assistant, its tools, and its refusal behavior are described on the [Ask assistant](/bot/ask-assistant/) page.
