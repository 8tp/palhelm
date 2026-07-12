---
title: Keys and redaction
description: How integration keys are created, stored, and revoked, the key format, and the redaction model that keeps token responses safe to post in public.
sidebar:
  order: 2
---

This page covers the two things that make the Integration API safe to hand to a bot: how keys work, and what the API refuses to tell a key holder. Read both before you create a key.

## The key lifecycle

Keys are managed from the panel by an admin, on the Settings screen. The three operations map to three admin-only routes on the panel API (not the Integration API):

| Action | Route | Result |
|---|---|---|
| Create | `POST /api/v1/integration-keys` | Returns the new key once, plaintext |
| List | `GET /api/v1/integration-keys` | Labels, ids, timestamps. Never the key |
| Revoke | `DELETE /api/v1/integration-keys/{id}` | Marks the key revoked, immediately |

These routes require an admin session. A viewer gets `403`. A bearer key cannot reach them at all.

### Create: shown exactly once

Creating a key takes a label. The label is for your own bookkeeping (which bot, which script) and is trimmed to between 1 and 64 characters.

```json
POST /api/v1/integration-keys
{"label": "discord-bot"}
```

```json
HTTP/1.1 201 Created
Cache-Control: no-store

{
  "id": "a1b2c3d4",
  "label": "discord-bot",
  "createdAt": "2026-07-10T02:04:11Z",
  "lastUsedAt": null,
  "revokedAt": null,
  "key": "phk_a1b2c3d4_..."
}
```

The `key` field is the full plaintext bearer token. This is the **only** response anywhere in Palhelm that ever contains it. It is not logged, not audited, and cannot be retrieved again. Copy it into your bot's configuration the moment you see it. If you lose it, revoke the key and create a new one.

:::danger
There is no "show key again" button and no recovery path. The panel stores only a hash of the key, so it genuinely cannot reprint the plaintext. Store it immediately.
:::

### List: metadata only

The list endpoint returns each key's public id, label, and timestamps, newest first. It never returns the key or its hash. The `lastUsedAt` field tells you whether and when a key was last used, which is how you notice a leaked key still being exercised.

```json
GET /api/v1/integration-keys

[
  {
    "id": "a1b2c3d4",
    "label": "discord-bot",
    "createdAt": "2026-07-10T02:04:11Z",
    "lastUsedAt": "2026-07-10T02:31:55Z",
    "revokedAt": null
  }
]
```

### Revoke: immediate and permanent

Revoking a key takes effect on the very next request. There is no restart and no propagation delay. Revocation is soft: the row is kept forever with `revokedAt` set, so the audit trail (label, creation time, last use) survives. There is no un-revoke and no rename. To rotate a key, create a new one and revoke the old one.

```json
DELETE /api/v1/integration-keys/a1b2c3d4

{
  "id": "a1b2c3d4",
  "label": "discord-bot",
  "createdAt": "2026-07-10T02:04:11Z",
  "lastUsedAt": "2026-07-10T02:31:55Z",
  "revokedAt": "2026-07-10T03:00:00Z"
}
```

Revoking a key that is already revoked returns the original `revokedAt` and is otherwise a no-op. An unknown id returns `404 not_found`.

### The 100-key cap

At most 100 active (unrevoked) keys can exist at once. Creating past that limit returns `409 too_many_keys`. Revoked keys do not count toward the cap. This bound is enforced atomically, so two admins creating keys at the same moment cannot both slip past 100.

## Key format

A key has three parts joined by underscores:

```
phk_<id>_<secret>
```

- `phk_` is a fixed prefix. It makes keys easy to spot in a leaked config file and easy for secret-scanning tools to flag.
- `<id>` is 8 lowercase hex characters. This part is public and non-secret. It is the id you see in the list endpoint, in logs, and in audit events. It is how a key is named everywhere except the one create response.
- `<secret>` is 43 URL-safe base64 characters carrying 256 bits of randomness.

The whole string is 56 characters. In prose and examples throughout these docs the key is shown truncated, like this:

```
phk_a1b2c3d4_<43 more characters, shown once>
```

That is intentional. A full, valid-looking key never appears in documentation.

### How keys are stored and checked

The server stores only the SHA-256 hash of the full key string. There is no column that could hold the plaintext. On each request the server hashes the presented key and compares it in constant time against the stored hash for that id. An unknown id runs the same hash-and-compare against a dummy value, so a valid id and an invalid id take the same amount of work. Nothing about a failed request reveals whether the id existed.

Because the secret carries 256 bits of entropy, guessing a valid key is infeasible, and a fast hash like SHA-256 is the right tool: there is no low-entropy password to protect with a slow key-derivation function.

## The redaction model

Assume every response from this API ends up pasted into a public Discord channel. That single assumption drives the whole design. The rule is **viewer-minus**: a key holder sees a strict subset of what a signed-in read-only viewer sees, and never more.

Redacted fields are **absent** from the response, not set to `null`. Omission is unambiguous: a field that is gone was removed on purpose, not "temporarily unknown".

### What is removed and why

| Field | On the panel (viewer) | On the Integration API | Why |
|---|---|---|---|
| `uid` | shown | shown | Save-derived join key, not a platform credential |
| `name` | shown | shown | The in-game display name is the point of the API |
| `steamId` / platform ids | shown | **removed** | Durable real-account identity; enables profile lookup and correlation |
| `accountName` | shown | **removed** | Platform account name; same correlation risk |
| `ping` | shown | **removed** | Network telemetry hinting at location; no bot value |
| player `location` (live) | shown | **removed** | Real-time position is a stalking and raid-targeting primitive |
| `banned` | shown | **removed** | Moderation state is the operator's business |
| `whitelisted` | shown | **removed** | Local operator annotation; no reader value |
| `sessions` (player detail) | shown | **removed** | Per-person connection timeline is surveillance material |
| `worldGuid` (`/server`) | shown | **removed** | Infrastructure identifier that fingerprints the deployment |
| `panelVersion` (`/server`) | shown | **removed** | Advertises the exact build to leaked-key holders |
| base `location` (guilds) | shown | shown | Bases are persistent, communal, already discoverable in-game |
| `level`, `playtimeSec`, `firstSeenAt`, `lastSeenAt` | shown | shown | Coarse progress and presence; leaderboard staples |

Note the deliberate asymmetry on location: a live player's position is removed, but a guild base location is kept. A base is a fixed, shared structure everyone on the server already knows about, and plotting bases is the stated purpose of the `/map` endpoint. A person's live position is a moving target on a specific human, so it never leaves the authenticated panel.

### Before and after

Here is the same player as the panel's viewer sees them, then as the Integration API returns them.

Panel API (`GET /api/v1/players`), viewer session:

```json
{
  "uid": "a3f1c8e290b74d51a6e0f2c9b1d34e57",
  "steamId": "7656-EXAMPLE-REDACTED",
  "accountName": "kestrel_acct",
  "name": "Kestrel",
  "online": true,
  "level": 48,
  "guildId": "7c2d9e14",
  "guildName": "Lakeside Co-op",
  "ping": 42,
  "location": {"x": -128340.0, "y": 205117.5},
  "firstSeenAt": "2026-06-02T18:41:07Z",
  "lastSeenAt": "2026-07-10T01:58:22Z",
  "playtimeSec": 384210,
  "banned": false,
  "whitelisted": true
}
```

Integration API (`GET /api/integration/v1/players`), bearer key:

```json
{
  "uid": "a3f1c8e290b74d51a6e0f2c9b1d34e57",
  "name": "Kestrel",
  "online": true,
  "level": 48,
  "guildId": "7c2d9e14",
  "guildName": "Lakeside Co-op",
  "firstSeenAt": "2026-06-02T18:41:07Z",
  "lastSeenAt": "2026-07-10T01:58:22Z",
  "playtimeSec": 384210,
  "captureTotal": 1287,
  "uniquePalsCaptured": 143,
  "paldeckUnlocked": 161
}
```

`steamId`, `accountName`, `ping`, `location`, `banned`, and `whitelisted` are gone. Nothing is nulled out; the keys simply do not exist.

### How the removal is enforced

Each endpoint builds its response from a dedicated struct whose fields are exactly the ones allowed to leave. There is no code path from a sensitive stored field into any integration response. Adding a new field to the surface means editing one of those structs, which the redaction tests watch. A field added to a shared internal type for the panel UI cannot silently appear here.

Whole categories of the panel are simply not present on this surface at all: configuration, the console, backups, the whitelist, and every login and session route. None of them is reachable with a bearer key.
