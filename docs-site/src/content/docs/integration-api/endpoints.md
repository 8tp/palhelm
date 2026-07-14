---
title: Endpoints
description: All nine Integration API endpoints with example requests and responses, using fictional data.
sidebar:
  order: 3
---

This page lists all nine endpoints with an example request and a realistic response for each. Every route is a `GET`, returns JSON, and uses RFC 3339 UTC timestamps. The data below is fictional. For the envelope fields (`data`, `lastParseAt`, `formatDrift`, `nextCursor`) see the [Overview](/integration-api/overview/); for cursors and rate limits see [Pagination and limits](/integration-api/pagination-and-limits/).

Every request carries the bearer key:

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  https://panel.example.com/api/integration/v1/players
```

The nine endpoints:

| Route | Paginated | Save-derived |
|---|---|---|
| `GET /players` | yes | yes |
| `GET /players/{uid}` | no | yes |
| `GET /pals` | yes | yes |
| `GET /guilds` | no | yes |
| `GET /map` | no | no |
| `GET /server` | no | no |
| `GET /metrics/current` | no | no |
| `GET /world/summary` | no | no |
| `GET /events` | no | no |

## GET /players

A keyset-paginated roster of every known player, ordered by `uid`. Add `?online=true` to return only players the poller currently sees online. The optional progression fields (`captureTotal`, `uniquePalsCaptured`, `paldeckUnlocked`) are decoded from each player's save and are omitted, never zero-filled, when that save cannot be read.

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  "https://panel.example.com/api/integration/v1/players?limit=2"
```

```json
{
  "data": [
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
    },
    {
      "uid": "b80d47f1a9c24e6fb35c81902ad7e6f4",
      "name": "VossR",
      "online": false,
      "level": 51,
      "guildId": "7c2d9e14",
      "guildName": "Lakeside Co-op",
      "firstSeenAt": "2026-05-28T09:12:44Z",
      "lastSeenAt": "2026-07-09T23:05:10Z",
      "playtimeSec": 452880
    }
  ],
  "lastParseAt": "2026-07-10T02:00:00Z",
  "formatDrift": false,
  "nextCursor": "djF8YjgwZDQ3ZjFhOWMyNGU2ZmIzNWM4MTkwMmFkN2U2ZjQ"
}
```

VossR shows the omitted-progression case: their save-derived capture counts could not be decoded, so those three fields are absent rather than zero.

## GET /players/{uid}

One player, keyed by the `uid` from `/players`, plus that player's full pal roster. The `uid` path segment must match `^[0-9a-fA-F-]{1,36}$`; anything else returns `404` without touching the store. An unknown but well-formed `uid` also returns `404 not_found`. The `pals` array uses the same fields as [`/pals`](#get-pals) minus the owner columns.

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  https://panel.example.com/api/integration/v1/players/a3f1c8e290b74d51a6e0f2c9b1d34e57
```

```json
{
  "data": {
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
    "paldeckUnlocked": 161,
    "pals": [
      {
        "instanceId": "e11d2a4c8b0f43a7920c6d5e1f83b7a0",
        "characterId": "Lamball",
        "displayName": "Lamball",
        "level": 12,
        "isAlpha": false,
        "isLucky": false,
        "inParty": true,
        "partySlot": 0,
        "boxPage": null,
        "boxSlot": null,
        "placement": "party",
        "baseId": null,
        "hp": 340.0,
        "gender": "female",
        "talents": {"hp": 40, "melee": 0, "shot": 15, "defense": 70},
        "passiveSkillIds": ["Rare", "Legend"],
        "equippedSkillIds": ["PowerShot", "IceMissile"]
      },
      {
        "instanceId": "5c9a0e73f1b84d62a7e30c1948f6d2b5",
        "characterId": "BOSS_Foxparks",
        "displayName": "Foxparks",
        "level": 30,
        "isAlpha": true,
        "isLucky": false,
        "inParty": false,
        "partySlot": null,
        "boxPage": 0,
        "boxSlot": 4,
        "placement": "box",
        "baseId": null,
        "hp": 615.0,
        "gender": "male",
        "talents": {"hp": 80, "melee": 55, "shot": 20, "defense": 10},
        "passiveSkillIds": [],
        "equippedSkillIds": ["FireBall"]
      }
    ]
  },
  "lastParseAt": "2026-07-10T02:00:00Z",
  "formatDrift": false
}
```

The second pal shows a `BOSS_` character id resolving to its base species name (`Foxparks`) with `isAlpha` set to `true`. Placement is derived and safe: `party`, `box`, `base`, or `unknown`. Raw container GUIDs are never exposed.

## GET /pals

A keyset-paginated roster of every pal in the world, ordered by `instanceId`, with owner attribution joined in so a bot does not have to call `/players/{uid}` per pal. In addition to the identity, level, placement, and per-instance fields shown above, each row carries owner provenance.

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  "https://panel.example.com/api/integration/v1/pals?limit=1"
```

```json
{
  "data": [
    {
      "instanceId": "0af3b1c27d9e4658a10b3f2c4d5e6f78",
      "characterId": "Depresso",
      "displayName": "Depresso",
      "level": 22,
      "isAlpha": false,
      "isLucky": true,
      "inParty": false,
      "partySlot": null,
      "boxPage": null,
      "boxSlot": null,
      "placement": "base",
      "baseId": "9f4e2b71",
      "ownerUid": "c14e7a90d2b34f68b5a1c8e0f3d29b47",
      "ownerName": "mika_o",
      "ownerSource": "last_observed",
      "ownerResolved": true,
      "hp": 505.0,
      "gender": "male",
      "talents": {"hp": 25, "melee": 30, "shot": 5, "defense": 45},
      "passiveSkillIds": ["Workaholic", "Serious"],
      "equippedSkillIds": ["PoisonBlast"]
    }
  ],
  "lastParseAt": "2026-07-10T02:00:00Z",
  "formatDrift": false,
  "nextCursor": "djF8MGFmM2IxYzI3ZDllNDY1OGExMGIzZjJjNGQ1ZTZmNzg"
}
```

Owner fields:

- `ownerUid` and `ownerName` identify the attributed player. `ownerName` is an empty string when the owner is unresolved or has no name.
- `ownerSource` is `save`, `personal_container`, `last_observed`, or `unresolved`. `last_observed` is honest historical attribution while a pal is deployed outside a personal container. It is not proof of current possession.
- `ownerResolved` is `true` when `ownerUid` joins a current player row. Check `ownerSource` for how certain the attribution is.

`placement: "base"` with a non-null `baseId` is emitted only when the pal's container exactly matches a decoded base; `baseId` joins `guilds[].bases[].id`. An unmatched container stays `unknown` rather than being guessed. Box pages hold 30 slots each and both page and slot are 0-based.

:::note
Work suitability and base-combat stats are deliberately not in these responses. They are version-pinned species metadata. Join them client-side by `characterId` against your own Paldeck data.
:::

## GET /guilds

Every guild with its members and bases. Not paginated: guild counts are small and the response is bounded by save size. Base `location` is exposed here (bases are communal and already discoverable in-game); live player positions are not.

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  https://panel.example.com/api/integration/v1/guilds
```

```json
{
  "data": [
    {
      "id": "7c2d9e14",
      "name": "Lakeside Co-op",
      "adminUid": "b80d47f1a9c24e6fb35c81902ad7e6f4",
      "memberCount": 2,
      "members": [
        {"uid": "a3f1c8e290b74d51a6e0f2c9b1d34e57", "name": "Kestrel"},
        {"uid": "b80d47f1a9c24e6fb35c81902ad7e6f4", "name": "VossR"}
      ],
      "bases": [
        {"id": "9f4e2b71", "location": {"x": -128340.0, "y": 205117.5}, "level": 3}
      ]
    },
    {
      "id": "1a5b8c02",
      "name": "Dawnbreakers",
      "adminUid": "d72f3e10a9c84b56b0e1a2c3f4d5e6a7",
      "memberCount": 1,
      "members": [
        {"uid": "d72f3e10a9c84b56b0e1a2c3f4d5e6a7", "name": "HaruQ"}
      ],
      "bases": [
        {"id": "3c8d1f60", "location": {"x": 44210.0, "y": -88750.0}, "level": 2}
      ]
    }
  ],
  "lastParseAt": "2026-07-10T02:00:00Z",
  "formatDrift": false
}
```

## GET /map

Dataset metadata for plotting base coordinates on your own map. This returns transforms and layer bounds, not tile images. Tile art is not served through this API.

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  https://panel.example.com/api/integration/v1/map
```

```json
{
  "data": {
    "source": "thgl",
    "gameVersion": "1.0",
    "fetchedAt": "2026-06-30T14:00:00Z",
    "notes": "World coordinates map to tile pixels via each layer's transform.",
    "layers": [
      {
        "id": "world",
        "label": "World",
        "format": "webp",
        "tileSize": 256,
        "minZoom": 0,
        "maxZoom": 6,
        "transform": {"a": 0.002, "b": 0.0, "c": 0.0, "d": -0.002},
        "bounds": [[-500000.0, -500000.0], [500000.0, 500000.0]]
      }
    ]
  }
}
```

To place a base marker, apply the layer `transform` to the base `location` from `/guilds`. A point `(x, y)` maps to `(a*x + c*y, b*x + d*y)` in the layer's coordinate space.

## GET /server

Server status, served from the poller's cached last-successful snapshot. This endpoint never makes a live call to the game server, so a busy key holder cannot generate load on the process running Palworld.

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  https://panel.example.com/api/integration/v1/server
```

```json
{
  "data": {
    "name": "palworld",
    "description": "Friendly co-op server. Be kind.",
    "version": "v1.0.0.100427",
    "state": "running",
    "uptimeSec": 183642,
    "save": {
      "state": "ok",
      "formatDrift": false,
      "lastParseAt": "2026-07-10T02:00:00Z",
      "players": 5,
      "pals": 812,
      "guilds": 2
    }
  }
}
```

When no snapshot exists yet, or the poller currently reports the game server unreachable, this returns `200` with `state: "unreachable"` and empty or zero live fields, never a `5xx`. `save.state` is `drift` when the last parse hit format drift, `unknown` before the first completed parse, and `ok` otherwise. The `worldGuid` and `panelVersion` a signed-in viewer would see here are removed.

## GET /metrics/current

Current performance and uptime telemetry. No identity content.

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  https://panel.example.com/api/integration/v1/metrics/current
```

```json
{
  "data": {
    "fps": 58.4,
    "fpsAvg": 59.1,
    "frameTimeMs": 17.1,
    "players": 5,
    "maxPlayers": 16,
    "day": 214,
    "uptimeSec": 183642,
    "baseCamps": 6
  }
}
```

## GET /world/summary

Capability, freshness, FPS, and aggregate actor counts from the optional Palworld 1.0
game-data poller. This route reads Palhelm's one shared cache and never calls the game server
on demand. It deliberately contains no actor names, IDs, guilds, health, actions, trainer
links, or locations.

```json
{
  "data": {
    "state": "ready",
    "capturedAt": "2026-07-14T18:00:00Z",
    "lastAttemptAt": "2026-07-14T18:00:00Z",
    "fps": 57.4,
    "fpsAvg": 55.8,
    "counts": {
      "players": 3,
      "partyPals": 7,
      "basePals": 31,
      "wildPals": 86,
      "npcs": 12,
      "palBoxes": 3,
      "unknown": 0
    }
  }
}
```

`state` is one of `disabled`, `pending`, `ready`, `stale`, `unsupported`,
`unauthorized`, or `unavailable`. Consumers should display the state and `capturedAt`
instead of presenting cached observations as current.

## GET /events

A bounded window of recent public activity. Only three kinds of event survive the redaction: sanitized join and leave lines, a generic `Backup completed`, and four allowlisted system transitions about REST reachability and save-format drift. Panel, config, and audit events are discarded, and there is no `meta`, no cursor, and no kind filter. Use `?limit=` (default 50, min 1, max 100).

```bash
curl -H "Authorization: Bearer phk_a1b2c3d4_..." \
  "https://panel.example.com/api/integration/v1/events?limit=4"
```

```json
{
  "data": [
    {"at": "2026-07-10T01:58:22Z", "kind": "join", "message": "Kestrel joined"},
    {"at": "2026-07-10T01:40:03Z", "kind": "leave", "message": "tessellate left"},
    {"at": "2026-07-10T01:30:00Z", "kind": "backup", "message": "Backup completed"},
    {"at": "2026-07-10T00:12:41Z", "kind": "system", "message": "world save format drift resolved"}
  ]
}
```

The four allowlisted system messages are `Palworld REST API is reachable`, `Palworld REST API is unreachable`, `world save format drift detected`, and `world save format drift resolved`. Any other system text is dropped.

## Errors

Every error uses the same envelope:

```json
{"error": {"code": "not_found", "message": "Player not found."}}
```

The codes you can see on these endpoints are `unauthorized` (401), `rate_limited` (429), `not_found` (404), `invalid_limit`, `invalid_cursor`, `invalid_request` (all 400), `method_not_allowed` (405), and `internal_error` (500). The 500 message is always generic; it never leaks internal error text, file paths, SQL, or key material. The 401 and rate-limit behavior are covered in [Pagination and limits](/integration-api/pagination-and-limits/).
