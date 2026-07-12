---
title: Ask assistant
description: How the /ask command works, including its bounded tool loop, read-only tools, built-in game data, optional web search, refusal scope, and provider requirements.
sidebar:
  order: 4
---

This page explains how the `/ask` command works: the bounded tool loop it runs, the read-only tools it can call, the built-in Palworld 1.0 data, the optional cited web search, what it refuses to do, and the requirement on the AI provider. `/ask` is optional and stays disabled until you configure it. See [Configuration](/bot/configuration/) for the settings.

## What /ask is

`/ask <question> [private]` is a read-only Palworld assistant. It answers questions about the game and about your live server using a small set of read-only tools. Set `private` to keep the reply visible only to you.

The command is disabled until `OPENROUTER_API_KEY` is set. When it is not configured, `/ask` reports that it is unavailable rather than guessing.

## The bounded tool loop

Each question runs a short, capped loop. The model may make up to four calls and up to three rounds of tool use before it must answer. Every model round trip has a deadline set by `AI_TIMEOUT_MS`. Usage is capped by `AI_DAILY_REQUEST_LIMIT` per day and by a per-user cooldown from `AI_COOLDOWN_SEC`.

While the loop runs, the reply shows live status such as "Searching the web…" so you can see what it is doing.

## The read-only tools

The assistant can call roughly 19 tools. All of them are read-only. There are three sources behind them:

- **Live server snapshot.** Deterministic queries over the same shared five-minute snapshot the rest of the bot reads from. See [Notifications and history](/bot/notifications-and-history/).
- **Pinned Palworld 1.0 knowledge.** A disk-cached, version-pinned mechanical dataset. It covers pal search and details, work suitability, stats, active-skill power and cooldowns, guaranteed passive traits, wild level ranges, movement, food and stamina, exact breeding pairs, reverse breeding lookup, and owned-worker recommendations.
- **Optional web search.** A Palworld-scoped read-only search, described below.

:::note
The assistant has no session, admin, backup, announcement, shell, or mutation tools. It cannot change anything on the server or in Discord. It can only read.
:::

## Guarding against invented pals

The assistant validates pal names against the Paldeck before using them, so it does not invent species that do not exist. This is a hard guard, not a hint.

## Optional web search with citations

Some topics are not in the pinned dataset: partner skills, item drops, spawn coordinates, recipes, and technology unlocks. When web search is available, the assistant searches for these, cites a relevant result, and treats the returned snippets as untrusted and possibly version-sensitive.

Web search is enabled only when `SEARXNG_URL` points at a self-hosted SearXNG instance. The documentation uses `https://searxng.example.com` as the placeholder. When search is unavailable or returns nothing, the assistant reports the gap instead of guessing.

Successful searches use a bounded, restart-safe cache written with owner-only file permissions. When search is briefly offline, the assistant may reuse an explicitly labeled last-good result.

For a handful of common material and technology questions, such as Meteorite Fragments, Dog Coins, and Ancient Civilization materials, the assistant first consults a small attributed, versioned CC BY-SA corpus and avoids a live search. See the bot's third-party notices for the attribution.

## Refusal scope

The assistant is strictly Palworld only. It refuses off-topic questions. It refuses to guess when it lacks grounded data, and it will tell you when a topic is outside its pinned dataset and search is unavailable.

## Provider requirement

`/ask` sends requests through OpenRouter. Use a provider that denies data collection and requires zero data retention. This is a requirement, not a preference: the assistant is meant to run without your questions being retained by the model provider.

## Example

:::note
TRANSCRIPT. Illustrative example with fictional content.
:::

```
/ask question: what does meteorite ore do?

Assistant: Meteorite Fragments are a late-game material used to craft
high-tier ammunition and gear. You gather them from meteor-strike sites
and certain ore deposits.

Source: https://searxng.example.com/ (Palworld wiki result)
```
