# Palhelm REST API (v1)

Base path `/api/v1`. JSON everywhere. Times are RFC 3339 UTC. Sizes are bytes.
Auth: `POST /auth/login` sets an HttpOnly session cookie (JWT). Roles: `admin`, `viewer`.
Every endpoint requires a session unless marked *public*. Mutating endpoints require `admin`.
Errors: `{"error": {"code": "string", "message": "human sentence", ...details}}` with 4xx/5xx.
Operation-specific recovery details, such as Config's `manualCommand`, stay inside `error`.

## Auth
| Method | Path | Notes |
|---|---|---|
| POST | `/auth/login` | *public*. `{password}` → sets cookie, returns `{role}` |
| POST | `/auth/logout` | clears cookie |
| GET  | `/auth/session` | `{role, username}` or 401 |

## Server
| GET | `/server` | `{name, description, version, worldGuid, state, uptimeSec, panelVersion}` |
| GET | `/server/health` | `{rest: "ok"\|"error", rcon: "ok"\|"error", save: {state, lastSyncAt}}` |
| POST | `/server/announce` | `{message}` |
| POST | `/server/save` | trigger world save |
| POST | `/server/shutdown` | `{waitSec, message, countdown: bool}` — countdown=true sends staged warnings (10/5/1 min, 30/10 s) then graceful shutdown |
| POST | `/server/shutdown/cancel` | cancels a pending countdown (before the final REST call) |

## Metrics
| GET | `/metrics/current` | `{fps, fpsAvg, frameTimeMs, players, maxPlayers, day, uptimeSec, baseCamps}` |
| GET | `/metrics/history?window=1h\|24h\|7d&step=auto` | `{series: {t: [unix...], fps: [...], frameTimeMs: [...], players: [...]}}` raw ≤24h, 1-min rollups beyond |

## Players
| GET | `/players` | union of live + save-derived: `[{uid, steamId, name, accountName, online, level, guildId, guildName, ping, location: {x,y}\|null, firstSeenAt, lastSeenAt, playtimeSec, banned, whitelisted, captureTotal?, uniquePalsCaptured?, paldeckUnlocked?}]`; optional progression is decoded from the player save |
| GET | `/players/{uid}` | detail incl. save-derived Pal placement and individual HP, gender, talents, passive IDs, and equipped-skill IDs; `sessions: [...]`. The three placement numbers are nullable. |
| POST | `/players/{uid}/kick` | `{message?}` |
| POST | `/players/{uid}/ban` | `{message?}` |
| POST | `/players/{uid}/unban` | |
| GET/PUT | `/whitelist` | Legacy path for the local player-annotation ledger: `[{steamId, name?}]`; PUT replaces. It does **not** enforce who may join. |

## Guilds
| GET | `/guilds` | `[{id, name, adminUid, memberCount, members: [{uid,name}], bases: [{id, location:{x,y}, level}]}]` |

## World / save data
| GET | `/world` | `{day, lastParseAt, parseDurationMs, stats: {players, pals, guilds, skippedProps}, formatDrift: bool}` |
| POST | `/world/parse` | force re-parse now (409 if already running) |

## Console (RCON)
| POST | `/console/exec` | `{command}` → `{output}` (audit-logged) |
| GET  | `/console/log?limit=200` | this panel's RCON session history `[{at, user, command, output, isError}]` |
| GET/POST/DELETE | `/console/saved` · `/console/saved/{id}` | saved commands `{id, name, command}` |

## Backups
| GET | `/backups` | `[{id, file, createdAt, sizeBytes, trigger: "scheduled"\|"manual"\|"pre-restore"\|"imported", worldDay?}]` |
| POST | `/backups` | create now → 201 `{id,...}` |
| GET | `/backups/{id}/download` | tar.gz stream |
| GET | `/backups/{id}/contents` | `[{path, sizeBytes, modifiedAt}]` |
| POST | `/backups/{id}/restore/dry-run` | diff vs live save: `{changes: [{path, kind: "add"\|"modify"\|"delete", fromSize?, toSize?}], requiresStop: true}` |
| POST | `/backups/{id}/restore` | `{confirm: "RESTORE"}`. Refuses (409) whenever the game REST API is reachable; the server must be stopped. Always takes a `pre-restore` backup first. |
| DELETE | `/backups/{id}` | |
| GET/PUT | `/backups/schedule` | `{enabled, everyMinutes, keepDays, nextRunAt}`. `nextRunAt` is the persisted actual deadline or `null` while disabled; PUT resets it immediately. |

## Config (game settings via compose env)
| GET | `/config` | `{source: "compose"\|"ini", composeFile?, service, version?, capabilities: {write, apply}, manualCommand, settings: [{key, value, effectiveValue, type, group, default, pending, editable, readOnly}]}`. Values/defaults are native string, number/integer, or boolean values. Password values are always the write-only placeholder `"•••"`. `effectiveValue` comes from live REST `/settings` with ini fallback. |
| PUT | `/config` | `{version, changes: {KEY: value}}` → atomically writes only the compose environment mapping and returns the complete updated Config document. `409 config_conflict` when the file changed since GET; `409 config_read_only` when safe directory-bind writes are unavailable. |
| GET | `/config/raw` | current `PalWorldSettings.ini` text (admin only, read-only) |
| POST | `/config/apply` | intentionally returns 501 in v0.3.0 with `error.manualCommand`; run it from the host directory containing the compose file. |

`editable` means the setting and detected deployment support PUT; `readOnly` is its exact
inverse. This replaces the ambiguous v0.2 `managed` field. `capabilities.write.reason`
explains missing/inaccessible/single-file mounts. One-click apply stays unavailable because a
Compose command inside the Palhelm container cannot safely preserve arbitrary host project
identity and relative host bind paths.

## Events
| GET | `/events?limit=100&kind=` | `[{at, kind: "join"\|"leave"\|"backup"\|"system"\|"panel"\|"config", message, meta}]` |
| GET | `/events/stream` | Server-Sent Events: `event: metrics|players|event` payloads for live UI |

## Integration keys (admin)
Manage bearer credentials for the separate Integration API below. Admin only (viewer gets
`403 forbidden`, matching every other admin route — not 404). Every response on these three
routes, including the `201`, carries `Cache-Control: no-store`.

| Method | Path | Request | Response |
|---|---|---|---|
| POST | `/integration-keys` | `{label}` — required; trimmed; 1–64 chars after trim; control characters rejected → `400 invalid_request` | `201` `{id, label, createdAt, lastUsedAt: null, revokedAt: null, key: "phk_..."}` — **the only response that ever contains the plaintext key.** `409 too_many_keys` at the 100-active-key cap |
| GET | `/integration-keys` | | `200` `[{id, label, createdAt, lastUsedAt, revokedAt}]`, newest first; never contains `key` or a digest |
| DELETE | `/integration-keys/{id}` | | `200` with the updated record (`revokedAt` set). Idempotent — revoking an already-revoked key returns the original `revokedAt`. Unknown id → `404 not_found` |

Revocation is soft: the row is retained forever (with `revokedAt` set) for the audit trail; there
is no hard delete and no un-revoke. Re-issuing means creating a new key. The plaintext key is
shown exactly once, in the create response — copy it immediately, there is no way to retrieve it
again.

## Integration API (bearer tokens)
A separate, read-only surface at **`/api/integration/v1`** for Discord bots, dashboards, and
scripts — distinct from the session-cookie API above and never reachable with a session cookie.
It is a dedicated GET-only chi sub-router: any non-`GET` method on a real path returns
`405 method_not_allowed` with `Allow: GET`, and any path — real or not — returns a uniform `401`
without a valid key (auth runs before routing, so probing cannot even discover which paths
exist).

**Auth.** `Authorization: Bearer phk_<8 lowercase hex>_<43 base64url chars>` (56 characters
total). Keys are created via `POST /api/v1/integration-keys` above; the plaintext is shown
**exactly once**, at creation, and is never retrievable again — treat it like a password. Every
failure mode (missing header, malformed header, unknown key id, wrong secret, revoked key)
returns the identical `401 {"error":{"code":"unauthorized","message":"A valid API key is
required."}}` plus `WWW-Authenticate: Bearer`, so no response variant can be used as an oracle to
probe key validity. **Serve this API over TLS at the network edge and treat keys as passwords —**
a bearer key sent over cleartext HTTP is readable by anyone on the path, and the panel's
trusted-network-edge posture (see ARCHITECTURE.md) is the operating assumption, not a substitute
for TLS termination.

**Endpoints.** All are `GET`, JSON, RFC 3339 UTC timestamps, sharing the envelope
`{"data": ..., "lastParseAt": ..., "formatDrift": ..., "nextCursor": ...}` — `lastParseAt`
and the always-present boolean `formatDrift` appear on save-derived endpoints (`/players`,
`/players/{uid}`, `/pals`, `/guilds`); `nextCursor` appears only on paginated endpoints.

| Route | Shape of `data` |
|---|---|
| `GET /players` | `[{uid, name, online, level, guildId, guildName, firstSeenAt, lastSeenAt, playtimeSec, captureTotal?, uniquePalsCaptured?, paldeckUnlocked?}]` — paginated; progression fields are omitted when the per-player save cannot be decoded; `?online=true` filters to the poller's current online set |
| `GET /players/{uid}` | one player object as above, plus `pals` with placement and the rich fields documented for `/pals`. Placement fields are always present: `partySlot`, `boxPage`, and `boxSlot` are numbers or `null`; `placement` is `party`, `box`, `base`, or `unknown`; `baseId` is the safe derived join to `guilds[].bases[].id` or `null`. A `{uid}` that fails `^[0-9a-fA-F-]{1,36}$` is `404` before any store lookup (never resolves as a SQL wildcard); unknown uid is also `404 not_found` |
| `GET /pals` | Bulk paginated roster. In addition to identity, level, rare flags, placement, and owner provenance, every row includes `hp`, normalized `gender`, `talents: {hp, melee, shot, defense}`, `passiveSkillIds`, and `equippedSkillIds`. `placement=base` is only emitted when the Pal's container exactly joins a decoded base WorkerDirector container; `baseId` then joins `/guilds[].bases[].id`. Raw container GUIDs are never exposed, and an unmatched container remains `unknown` rather than being guessed as a base. These are actual per-instance save observations; work suitability and base combat scaling remain version-pinned species metadata that clients join by `characterId`. `ownerSource` is `save`, `personal_container`, `last_observed`, or `unresolved`; `last_observed` is truthful historical attribution while a Pal is deployed outside a personal container, not proof of current possession. Box pages and slots are 0-based, with 30 slots per page. `BOSS_` IDs retain provenance but resolve to the base species name and set `isAlpha=true`. |
| `GET /guilds` | `[{id, name, adminUid, memberCount, members: [{uid, name}], bases: [{id, location: {x, y}, level}]}]` — not paginated |
| `GET /map` | `{source, gameVersion, fetchedAt, notes, layers: [{id, label, format, tileSize, minZoom, maxZoom, transform: {a,b,c,d}, bounds}]}` — dataset metadata for plotting base coordinates on your own map. **No tile images.** |
| `GET /server` | `{name, description, version, state, uptimeSec, save: {state, formatDrift, lastParseAt, players, pals, guilds}}` — served from the poller's cached last-successful snapshot, **never** a per-request call to the game server; `state: "unreachable"` with empty/zero live-server fields (never a 5xx) when no snapshot exists. `save.state` is `drift` when `formatDrift` is true, `unknown` before the first completed parse, and `ok` otherwise. |
| `GET /metrics/current` | `{fps, fpsAvg, frameTimeMs, players, maxPlayers, day, uptimeSec, baseCamps}` |
| `GET /events?limit=50` | `[{at, kind: "join"|"leave"|"backup"|"system", message}]` — bounded to 1–100 newest public events. Join/leave names are sanitized, backups are always `Backup completed`, and system text is restricted to REST reachability and save-format-drift transitions. No `meta`, panel/config/audit text, paths, or admin details. |

**Redaction (viewer-minus policy).** Assume every response is pasted into a public Discord
channel. Tokens see a strict subset of what the authenticated viewer sees — never more. These
fields **never** appear on this surface, in any endpoint, under any name: `steamId` /
platform account ids, `accountName`, `ping`, live player `location` (base locations under
`/guilds` **are** exposed — persistent, communal, already discoverable in-game), `banned`,
`whitelisted`, per-player `sessions`, `worldGuid`, `panelVersion`, and raw party/box container
GUIDs. Redacted fields are **absent** from the response, never `null`; pal placement exposes
only the derived boolean and indices.

**Pagination.** Keyset cursors, not offset — immune to duplicates/gaps across a save re-parse.
`/players` orders by `uid`; `/pals` orders by `instance_id`. Query params: `limit` (default 100,
min 1, max 500; out of range → `400 invalid_limit`) and `cursor` (opaque, from a previous page's
`nextCursor`; tampered or wrong-version → `400 invalid_cursor` — never construct one by hand).
`nextCursor` is always the key of the last row **returned**, and is `null` whenever the page
returned fewer than `limit` rows; an empty `data` array is always paired with a `null`
`nextCursor` (never the other way around) — clients stop when they see `null`. **Consistency
guarantee:** each page is internally consistent (a save re-parse swaps tables in one transaction,
so no page observes a half-replaced table); across pages, no row already present when pagination
started is ever duplicated or skipped, though rows created or deleted mid-walk may or may not
appear — the standard keyset contract. Use the per-response `lastParseAt` to detect a re-parse
landing mid-walk and restart if you need a single consistent snapshot.

Save format-drift transitions also create `kind: "system"` events: entering drift emits
`world save format drift detected` with the aggregate `skippedProps` count, and returning to a
clean parse emits `world save format drift resolved`. Steady-state parses do not repeat either
event.

**Conditional requests.** Every `200` carries a weak `ETag` (content-hash of the body); send it
back as `If-None-Match` (comma-separated list or `*` both accepted) to get a bodyless `304` with
the same `ETag` instead of re-downloading. This saves bandwidth, not query work — the server
still runs the rate-limited query — but it is nearly free for a bot that polls on a timer.

**Caching.** `Cache-Control: no-store` on every response on this surface, unconditionally — never
cache or persist a response body. **No CORS**: no `Access-Control-Allow-*` header is ever set on
this surface. A browser dashboard on another origin cannot call it directly; proxy the request
through your own server so the bearer key never lives in browser-held JavaScript.

**Rate limits.** 60 requests/minute per key by default (`PALHELM_INTEGRATION_RATE_LIMIT`
overrides it), a sliding window keyed on the key id — spoofed `X-Forwarded-For` has no effect,
since limiting is by token identity, not network identity. Exceeding it returns
`429 {"error":{"code":"rate_limited","message":"API key rate limit exceeded; retry later."}}`
with `Retry-After: <seconds>`.

**Errors.** Same envelope as the rest of the API: `{"error":{"code","message"}}`. Codes on this
surface: `unauthorized` (401, uniform), `rate_limited` (429), `not_found` (404),
`invalid_limit` / `invalid_cursor` / `invalid_request` (400), `method_not_allowed` (405, any
non-`GET` method inside the mount), `internal_error` (500 — generic message only; no upstream
error text, file paths, SQL, or key material is ever included).

Active-world backup selection uses the normalized world GUID returned by Palworld REST. A GUID
mismatch or ambiguous directory match fails closed. Only REST transport unavailability permits a
newest-directory fallback; the archive manifest and backup event include warning metadata when
that fallback is used. Manual, scheduled, pre-restore, dry-run, and restore operations share this
resolver and exclude conflicting backup operations.

## Meta
| GET | `/api/openapi.json` | *public* OpenAPI 3.1 document |
| GET | `/healthz` | *public* liveness for compose healthcheck |
