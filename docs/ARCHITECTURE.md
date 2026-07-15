# Palhelm Architecture

*Decision record — 2026-07-09. Validated against a live Palworld 1.0 dedicated server (v1.0.0.100427).*

## What Palhelm is

A self-hosted web admin panel for Palworld dedicated servers: one Docker image, one process,
no external database. It supersedes palworld-server-tool (PST) with a real design system,
metrics history, a backup browse/restore UI, an env-var-aware config editor, role-based auth,
and a documented REST API of its own.

## Stack

| Layer | Choice | Why |
|---|---|---|
| Backend | Go 1.26, `net/http` + chi router | Single static binary, PST-proven pattern |
| Storage | SQLite via `modernc.org/sqlite` (pure Go) | No CGO, single file, real SQL for metrics history |
| Frontend | React 19 + TypeScript + Vite; plain token CSS (no Tailwind, no UI kit) | The design-system CSS from `design/` ships verbatim — mockups stay authoritative |
| Charts | uPlot | Tiny, canvas-based, built for dense time series |
| Map | Custom pan/zoom tile layer (no Leaflet) | One less dep; tiles user-downloaded, never shipped |
| Distribution | Single Docker image; frontend embedded via `go:embed` | Compose-friendly, no runtime deps |

Schema changes go through a small embedded migration runner (`migrations/NNN_name.sql`, applied
in order inside `store.Open()`, version tracked in the existing `kv.schema_version` row, each
migration in its own `BEGIN IMMEDIATE` transaction with an in-transaction version re-read). It
opens a fresh database at the newest schema and an existing one incrementally; it fails closed
rather than opening a database whose version is newer than the binary knows about. v0.4.0 adds
the first migration since `001_init.sql`: `api_keys` (id, hash, label, timestamps), backing the
Integration API below. A v0.4 database opens cleanly under the v0.3 binary — the `api_keys` table
just sits unused — so rollback needs no manual step.

## Data sources (three channels, same as PST but 1.0-native)

1. **Official REST API** (`http://palworld:8212/v1/api/*`, HTTP basic auth `admin:<ADMIN_PASSWORD>`) —
   primary channel: info, players (with position/level/ping), metrics, settings, announce,
   kick/ban/unban, save, graceful `shutdown {waittime,message}`, force `stop`. Palworld 1.0's
   optional `game-data` snapshot is handled by a separately bounded, opt-in path because it can
   contain every loaded actor plus IP/platform identifiers; those two private fields are dropped
   during decode, then raw identifiers/actions are discarded during a one-time projection. Only
   aggregate counts plus at most 2,048 sanitized useful actors remain in the memory-only cache.
   The panel proxies it server-side; the admin password never reaches the browser.
2. **RCON** (`palworld:25575`, Source RCON) — console passthrough for the Console screen and
   the few things REST lacks (`TeleportToPlayer` etc.). Vanilla command set is small; we do not
   assume PalGuard.
3. **Save files** (read-only) — `Level.sav`, `LevelMeta.sav`, `Players/*.sav` mounted from the
   server's `Saved/` dir. Parsed on an interval + on demand into SQLite: players (incl. offline),
   pals, guilds, bases. `Players/` does not exist until the first player joins — handled as a
   normal state, not an error.

## The 1.0 save format (validated on this box, launch night)

- Container: `[u32 uncompressed_len][u32 compressed_len]"PlM" 0x31`, body is **Oodle Mermaid**
  (first block bytes `8C 0A`). Same container since v0.6; 1.0 did not change it.
  Old saves are `"PlZ"` + zlib (`0x31` single, `0x32` double) — we keep a zlib path for those.
- Body: standard `GVAS`, `++UE5+Release-5.1`. Generic property tree parses with pre-1.0 readers.
- Palworld-specific `RawData` blobs: the guild blob (`GroupSaveDataMap`) **changed in 1.0** —
  cheahjs/palworld-save-tools 0.24.0 (abandoned, Oct 2024) fails on it. The maintained
  **oMaN-Rod/palworld-save-tools** fork parses it fully (verified: 7/7 groups on the live save).
- Upstream watch list: oMaN-Rod fork commits; deafdudecomputers/PalworldSaveTools (shipped a
  confirmed 1.0 parser on launch night).

### Parser plan (Go, decode-only)

Pure-Go port, no Python, no CGO:

1. Container reader: header sniff → zlib (PlZ) or Oodle (PlM).
2. **Oodle decompression** via [purego] `dlopen` of `liboo2corelinux64.so.9`
   (`OodleLZ_Decompress`) with `RTLD_LOCAL`: the absolute path in `PALHELM_OODLE_LIB` takes
   precedence, then `<PALHELM_DATA_DIR>/liboo2corelinux64.so.9`; when neither exists it is
   downloaded atomically into the data dir and verified against a pinned SHA-256 before load.
   The lib is **never** committed to the repo or baked into the image (no redistribution right).
   Fallback validated: powzix/ooz decodes these streams bit-exact, but it is unlicensed —
   kept out of the tree.
3. GVAS reader: generic property tree (decode only), ported with oMaN-Rod's fork as reference.
4. Custom `RawData` decoders: only `CharacterSaveParameterMap.Value.RawData` (players + pals,
   discriminated by `IsPlayer`) and `GroupSaveDataMap` (guilds: id, name, members, base ids).
   Item/foliage/dungeon blobs stay opaque bytes — we don't need them and skipping them avoids
   the 1–3 GB RAM spikes the Python tools hit on large saves.
5. Tolerant by design: unknown properties are skipped with a counter, a failed sub-decoder
   degrades that feature (badge in UI: "save format drift"), never the whole panel.

## Backend layout

```
palhelm serve
 ├─ pollers
 │   ├─ metrics    every 5s   REST /metrics → SQLite ring (raw 24h, 1-min rollups 30d)
 │   ├─ players    every 15s  REST /players → session tracking (join/leave events)
 │   ├─ savesync   every 10m  Level.sav parse → players/pals/guilds/bases tables
 │   └─ game-data  every 30s  optional bounded live-actor snapshot → memory-only cache
 ├─ engines
 │   ├─ backup     schedule + manual; REST-GUID-resolved SaveGames/0/<world>; retention policy;
 │   │             browse (list contents), restore = dry-run diff first, then guided swap
 │   │             (requires explicit "server is stopped" confirmation flow)
 │   ├─ config     reads PalWorldSettings.ini for display; EDITS target the compose file's
 │   │             env block (see below)
 │   └─ actions    graceful shutdown w/ countdown broadcasts, save, kick/ban, announce
 ├─ HTTP API  /api/v1/*  (JSON, documented in docs/API.md, OpenAPI spec served at /api/openapi.json)
 │   └─ /api/v1/integration-keys  admin-only bearer-key management (create/list/revoke)
 ├─ Integration API  /api/integration/v1/*  a separate, GET-only, bearer-token-authenticated
 │   sub-router (players, pals, guilds, map, server, metrics, aggregate world summary) for bots/dashboards/scripts —
 │   viewer-minus redaction, keyset pagination, ETags, per-key rate limiting; never reachable
 │   with a session cookie, and the session API never reachable with a bearer key
 └─ embedded SPA at /
```

## Config editing and the env-var trap

The `thijsvanloef/palworld-server-docker` image **regenerates `PalWorldSettings.ini` from
compose env vars on every boot**. A config editor that writes the ini is silently reverted.
Palhelm therefore:

- Shows effective settings read-only from the REST `/settings` endpoint + the ini.
- Edits are written to the **compose file's `environment:` block** (compose file mounted at
  `PALHELM_COMPOSE_FILE`, service name `PALHELM_GAME_SERVICE`). A YAML-surgical editor changes
  only the targeted keys, preserving comments/ordering.
- Atomic replacement requires a **writable directory bind**, not a single-file bind. Operators
  mount a narrowly scoped host directory (for example `./palhelm-compose:/compose`) owned by the
  panel UID and place `docker-compose.yml` inside it. Startup probing verifies that the file is
  regular, is not itself a mount point, and that the parent supports sibling create + rename;
  otherwise Config remains explicitly read-only.
- Updates are serialized and use a SHA-256 content version from GET as a compare-and-swap token,
  so concurrent panel updates merge safely and external edits are rejected instead of overwritten.
  Changed strings are newly YAML-quoted, the resulting document is parsed, and its semantic tree
  is checked to ensure only requested environment scalars changed before atomic rename.
- Applying requires recreating the game container. v0.3.0 intentionally does **not** run Docker
  Compose from inside Palhelm: that cannot generally preserve the host project directory/project
  identity or relative host bind paths. The UI shows the exact manual command to run from the
  host directory containing the compose file. Pending-but-unapplied changes remain first-class state.

## Auth & roles

- `admin` (full control) and `viewer` (read-only) roles; passwords from env
  (`PALHELM_ADMIN_PASSWORD`, `PALHELM_VIEWER_PASSWORD` optional).
- Session JWT in an HttpOnly cookie; no credentials in localStorage; game-server admin
  password stays server-side always.
- Every mutating endpoint is role-gated server-side; the UI adapts (viewer sees no destructive
  affordances).
- Forwarded client IP/protocol headers are ignored unless the transport peer is in
  `PALHELM_TRUSTED_PROXIES`; login limiter state is bounded and expires. Sensitive Config/auth
  responses are non-cacheable, and both game passwords are write-only browser placeholders.
- A second, distinct principal type (`apiKeyPrincipal`, stored under its own context key) backs
  the Integration API's bearer tokens (`phk_<id>_<secret>`, SHA-256 stored, plaintext shown once).
  It is never mistaken for a session `principal`: bearer requests carry no cookie identity, and
  session-gated handlers can never observe a bearer request as authenticated. The bearer group is
  mounted as its own GET-only chi sub-router with its own middleware chain (parse → constant-time
  validate → per-key rate limit → handler) and its own uniform-401 auth-before-routing posture, so
  scope creep into `/api/v1/*` is structurally impossible, not just policy. Key management
  (`/api/v1/integration-keys`) stays admin-only session-cookie territory and is never reachable
  with a bearer key. There is no CORS support anywhere in the panel — no
  `Access-Control-Allow-*` header is ever set — so a browser-based dashboard on another origin
  must proxy Integration API calls server-side rather than call it directly from JavaScript.

## Backup safety

- Every operation resolves the active world from Palworld REST's normalized world GUID and fails
  closed on mismatch or ambiguity. Newest-directory fallback is allowed only while REST is
  transport-unavailable and is recorded in the manifest and event metadata.
- Manual, scheduled, pre-restore, dry-run, restore, delete, and retention operations exclude one
  another. Walk/copy/hash/extraction work observes application cancellation.
- Imported archives are limited by entry count, per-entry bytes, total expanded bytes, safe paths,
  and actual copied-byte verification. Schedule state persists its real deadline, resets on PUT,
  and becomes inactive immediately when disabled.

## Shutdown versus restart

Palhelm provides a cancellable graceful-shutdown countdown through the official REST API. It does
not call that operation a restart: once Palworld stops, the panel has no safe, general start
capability or post-shutdown observation path. Operators may configure an external supervisor or
container restart policy, but v0.3.0 neither verifies nor guarantees it.

## Map

The frontend uses a custom `CRS.Simple`-style pan/zoom renderer. Legacy tile sets retain the
old 256-square extent transform. Current layered datasets carry their provider's tile size,
zoom range, bounds, and `L.Transformation(a,b,c,d)` in `dataset.json`. THGL's transform is
applied in Palworld's axis-flipped order: horizontal input is save data Y and vertical input
is save data X. The in-game coordinate readout is `x=(dataY-158000)/459`,
`y=(dataX+123888)/459`, with the inverse used for fixtures.

Tiles are game-derived art and **not shipped**: `scripts/fetch-map-tiles.sh` downloads an
operator-selected pyramid to the data volume. Provider transform values must be preserved
verbatim; bounds are validation metadata, not a source for recomputing offsets. The map screen
has a proper empty state until tiles exist. Static points of interest are not shipped unless a
versioned, licensed, coordinate-verified dataset is available.

## Paldeck icons

Same pattern as map tiles: Pal preview icons are Pocketpair-derived art and **not shipped**.
`scripts/fetch-pal-icons.sh` reads the CharacterID roster straight from
paldeck.cc's rendered, current Paldeck by default (with the pinned backend roster plus
paldb.cc retained as a fallback), fetches each icon directly, and writes them into
`<dataDir>/pal-icons/<characterId-lowercased>.webp` (`.png` is also accepted for attributed,
operator-installed supplements). `GET /api/v1/paldeck/icon/{characterId}` serves them
case-insensitively with a long cache lifetime. Save-instance `BOSS_` prefixes resolve to base
species art, and a small explicit alias table handles known cosmetic/tower IDs; display names and
nicknames never participate in image identity. `GET /api/v1/paldeck/icon-dataset` reports the
sidecar provenance plus the union of the canonical roster and actual PNG/WebP filenames on disk,
so the frontend can use legitimate supplemental portraits without speculative requests. Both
routes are 404-safe — an unfetched or unrecognized id means the frontend falls back to initials.

The panel's Pal detail views join save `characterId` values to the checked-in
`frontend/src/data/palWorkSuitabilities.generated.ts` snapshot for species-level work
suitability. The artifact embeds its generated timestamp and pinned source commit URLs, so
standalone panel builds do not depend on the bot tree or a network fetch. Regenerate it only
from an explicitly selected normalized snapshot:

```sh
node scripts/generate-panel-pal-work-suitabilities.mjs path/to/pal-knowledge.json
```

The generator validates positive numeric levels and never derives work suitability from
nicknames or per-instance save fields. Unknown CharacterIDs remain unavailable.

## Security posture

- Panel binds wherever the operator publishes it (a private LAN or VPN address is strongly
  recommended). It assumes a trusted network edge, same as the official REST API guidance.
- No secrets in the repo or image; all runtime config via env.
- Backups and SQLite live in a mounted data volume.

## Non-goals (v1)

Multi-server management, PalGuard/PalDefender integration, mod management, in-place save
editing (give items/pals), anti-cheat. The API-first design leaves room for all of these.
