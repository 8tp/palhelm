# Spec: Palworld 1.0 game-data integration

Status: implemented behind the opt-in capability flag; enabled deployments use one shared poller.

## Upstream contract

Pocketpair documents `GET /v1/api/game-data` as a Basic-authenticated snapshot containing
server-local `Time`, `FPS`, `AverageFPS`, and all actors currently present in the world. Actor
variants are `Character` and `PalBox`; character unit types are `Player`, `OtomoPal`,
`BaseCampPal`, `WildPal`, and `NPC`. The wire format mixes casing (`level`, `HP`, `AI_Action`)
and represents `IsActive` as the strings `"true"` or `"false"`. Unknown fields and future actor
variants must be tolerated.

The endpoint is a transient observation, not a spawn table or persistent save replacement.
Loaded wild actors cannot establish where a species normally spawns, its time/weather/event
conditions, encounter rate, or capture probability.

Primary source: <https://docs.palworldgame.com/api/rest-api/game-data/>

## Ingestion and availability

- Disabled by default and capability-gated. A 404 means `unsupported` for the process lifetime;
  it does not degrade ordinary REST health.
- One immediate background fetch, then one shared interval. No browser, Integration API, bot,
  or AI request may call Palworld upstream directly.
- Request deadline is independently configurable. Responses are capped at 32 MiB and 250,000
  actor objects, redirects are refused, and non-finite coordinates/FPS are rejected.
- Each accepted generation is reduced once into aggregate counts plus a prioritized, sanitized
  projection capped at 2,048 players/party Pals/base Pals/PalBoxes. The raw DTO and response bytes
  are not cached. Integration summary requests are O(1); session requests copy only the bounded
  projection rather than rescanning the upstream actor array.
- Transient errors retain last-good data, mark it stale, and back off to a maximum ten-minute
  retry interval. Exact actors are withheld after a server-side freshness ceiling. Unsupported,
  unauthorized, and disabled states clear retained data; terminal failures stop polling until
  Palhelm restarts with corrected capability/configuration.
- A successful snapshot whose actor population falls below 25% of a previous generation of at
  least ten actors must repeat on the next poll before replacing last-good data. This avoids a
  one-off streaming/capture collapse masquerading as an authoritative empty world.
- The upstream non-ISO `Time` stays opaque. Palhelm's UTC `capturedAt` is the freshness authority.
- The authenticated snapshot carries bounded rollout diagnostics: the last request duration, the
  raw actor count of the last accepted generation, one allowlisted error category, and the current
  scheduled delay/next-attempt timestamp. The category is one of `none`, `collapsed`,
  `unreachable`, `unauthorized`, `unsupported`, `response`, `timeout`, `canceled`, or `unknown`;
  raw errors and response bodies are never retained or returned. State and projection truncation
  remain explicit top-level fields.

## Privacy boundary

The upstream response may contain IP addresses, platform user IDs, internal actor/trainer IDs,
names, guilds, exact positions, rotations, health, and raw AI/action state.

Palhelm does not declare `ip` or `userid` in its upstream Go DTO, so both are discarded while
JSON is decoded. Runtime actor/trainer IDs and raw class/action state exist only in the
short-lived decode result. For base workers, Pocketpair's compound
`runtime actor ID : save Pal ID` is parsed in memory and only its validated 32-hex save-Pal
component may survive after an exact roster join. The runtime component is discarded. Raw JSON
and raw actors are never cached, persisted, returned, or logged.

The authenticated session endpoint `/api/v1/world/snapshot` uses a second typed allowlist. It
shows exact locations only for players, party/base Pals, and PalBoxes; it omits raw IDs,
rotations, stages/classes/actions, and reduces activity to an allowlisted category. Wild Pals
and NPCs are counted but not sent as individual browser markers.

The map treats the ordinary REST player list as the roster authority. A non-truncated `ready`
game-data snapshot may replace coordinates only for one active actor with an exact, unique name
match. It never adds game-data-only players, hides unmatched REST players, or uses stale,
truncated, inactive, or ambiguous actors.

The bearer endpoint `/api/integration/v1/world/summary` is aggregate only. The separate
`/api/integration/v1/world/workers` endpoint contains only exact-linked save Pal identity,
base/owner provenance, HP percentage, and an allowlisted activity category. It contains no
locations, runtime IDs, guild/trainer names, or raw actions. Both inherit Integration API
authentication, ETag, no-store, and per-key rate limits. Poller rollout diagnostics remain
session-only.

## Implemented follow-on contracts

1. Live base workers join by the save-Pal half of Pocketpair's compound ID and carry a base only
   through the exact WorkerDirector/container relation. No location inference is used.
2. The panel map has worker/PalBox layers, exact-linked base health, and explicit unresolved data.
3. The Integration worker endpoint is location-free and excludes unresolved actors.
4. The bot has deterministic aggregate and exact-linked worker tools.
5. Aggregate activity/FPS history is retained for 30 days without actor identity, name, health,
   guild, or location data.

## Remaining external-data contract

Add spawn/catch guidance only from a licensed, versioned static dataset with source revision,
game version, map layer, coordinates/areas, and encounter conditions.
