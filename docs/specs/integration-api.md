# Spec: Integration API (v0.4.0)

Design record for Palhelm's programmatic, read-only Integration API: bearer-token access for
Discord bots, dashboards, and scripts. The data layer already exists (players, pals, guilds,
map dataset, metrics, server info); this spec covers auth, exposure, shaping, and hardening
only. It is the contract for Phase 2 implementers — every decision below is final, and any
deviation must update this document in the same commit. Do not deploy as part of this work.

Preserved v0.3.0 invariants: the game admin password never reaches any client; credential-shaped
values are write-only placeholders; sensitive responses use `Cache-Control: no-store`; rate
limiters are bounded, expiring, and fail closed (the `auth.go` login limiter is the model).

## Decisions at a glance

| Question | Decision |
|---|---|
| Namespace | Dedicated `/api/integration/v1`, its own mounted chi sub-router, GET-only |
| Key format | `phk_<id>_<secret>` — 56 chars total; `id` = 8 lowercase hex (public), `secret` = 43 base64url chars (32 random bytes) |
| Storage | `api_keys` table; SHA-256 of the full plaintext key; plaintext shown exactly once |
| Lookup | By public key id, then `subtle.ConstantTimeCompare` against the stored digest; dummy compare on unknown id |
| Revocation | Soft revoke via `DELETE /api/v1/integration-keys/{id}`; row retained and listed with `revokedAt` |
| Rate limit | 60 req/min per key id (sliding window), bounded map (1024 entries), fail-closed, `Retry-After` on 429 |
| Middleware order | parse header → validate key (constant-time, in-memory) → limiter keyed by key id → handler |
| Pagination | Keyset cursor over immutable unique keys (`uid`, `instance_id`); default limit 100, max 500; `nextCursor` = key of the last **returned** row, `null` when the query returned fewer than `limit` rows |
| Freshness | Weak content-hash `ETag` + `If-None-Match` 304 on every integration **200**; `lastParseAt` in SAV-derived envelopes |
| Caching | `Cache-Control: no-store` on **every** integration response (blanket) **and** on every `/api/v1/integration-keys` response (the `securityHeaders` path list gains that prefix) |
| CORS | **None in v0.4.0.** No `Access-Control-Allow-*` headers anywhere; browser dashboards must proxy server-side (§5) |
| `/server` source | Poller-cached last-successful Info snapshot; never a per-request upstream REST call (§4) |
| Redaction | Allowlist typed structs; `steamId`, `accountName`, `ping`, player `location`, `banned`, `whitelisted`, `sessions`, `worldGuid`, `panelVersion` never appear |
| Migrations | Numbered `NNN_name.sql` runner, version in existing `kv.schema_version`, one `BEGIN IMMEDIATE` transaction per migration with an in-transaction version re-read; fail closed when the DB version exceeds the binary's newest migration |

## 1. Namespace and routing

**Decision: a dedicated `/api/integration/v1` namespace.** Dual-auth on the existing `/api/v1`
paths was rejected: it would make every session handler conditionally redact based on principal
type — exactly the per-handler judgment this design forbids — and it would let one missed
conditional leak viewer-shaped data to tokens. A separate namespace also lets response envelopes
diverge (pagination, staleness metadata) without breaking the browser contract.

Routing structure in `routes()` (`backend/internal/server/server.go`):

```
r.Mount("/api/integration/v1", s.integrationRouter())
```

`integrationRouter()` returns a chi router that:

- applies the bearer middleware chain with `Use(...)` on the sub-router, so it runs before
  route resolution inside the group — unauthenticated probes get a uniform `401` whether or
  not the probed path exists (no surface enumeration without a key);
- registers **only** `Get(...)` handlers. No `Post`/`Put`/`Delete`/`Mount` of any other router
  is ever added to this group. Non-GET methods on valid paths return chi's method-not-allowed,
  overridden with the standard error envelope (`405`, `Allow: GET`) — after auth, like all
  responses in the group;
- sets `Cache-Control: no-store` on every response via group middleware (see §5);
- contains no reference to `s.auth`, the session cookie, or `adminOnly`. A bearer principal
  type (`apiKeyPrincipal{ID, Label string}`) is distinct from `principal` and is never placed
  under `authKey{}`, so no session-gated handler can ever observe a bearer request as
  authenticated. Fall-through into session or admin routes is structurally impossible: those
  routes live in a different subtree whose middleware only accepts the session cookie.

The root-level middleware stack (`middleware.RequestID, s.securityHeaders, s.recoverer,
s.requestLog` — server.go `routes()`) is registered with `r.Use(...)` on the root router and
therefore already applies to the mounted integration group. **Verified in code, not assumed**:
CSP, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and `Permissions-Policy`
reach every integration response with no new wiring. A test asserts this (§12).

The token surface consists of exactly eight routes (§4). Everything else — `/config`,
`/config/raw`, `/console/*`, `/backups/*`, `/whitelist`, `/auth/*`, the raw session
`/api/v1/events` feed, `/events/stream`,
`/world`, `/world/parse`, `/map-tiles/*`, `/paldeck/*` (including read-only icon serving —
decided **no**: Pocketpair-derived art must not become effectively public through tokens that
live in bot configs and public channels; the licensing posture in ARCHITECTURE.md keeps that
art session-gated) — is out of scope in any form and must never be reachable with a bearer
token. The full-router enumeration test (§12) proves this against the real router, not a list.

## 2. Token scheme

### 2.1 Key format

```
phk_<id>_<secret>
id     = 8 lowercase hex chars   (4 bytes from crypto/rand; public, non-secret)
secret = 43 base64url chars      (32 bytes from crypto/rand, RawURLEncoding, unpadded)
```

Total length is exactly 56 characters; charset is `[a-z0-9_]` for the prefix/id portion and
`[A-Za-z0-9_-]` for the secret. The `phk_` prefix makes keys grep-able in leaked configs and
secret-scanning tools. The key id is the stable non-secret handle used everywhere a key must be
named: logs, audit events, the Settings UI, limiter buckets, and `lastUsedAt` coalescing. The
id is generated at create time; on the (negligible) chance of a primary-key collision the
insert retries with a fresh id.

### 2.2 Storage and hashing

The server stores **only** `SHA-256(full plaintext key)` — the entire 56-char string including
prefix and id, which domain-separates the digest from any other SHA-256 use. Plaintext is
returned exactly once, in the create response; it is never persisted, never logged, never
audited, and is unreferenced after that response is written. (Go does not zero collected
strings, so "gone from memory instantly" is not claimed — "never persisted or emitted again"
is the enforceable invariant, and the one §12.8 tests.)

**Why plain SHA-256 and not a KDF:** stretching (bcrypt/argon2) exists to slow offline attacks
against *low-entropy* secrets. These keys carry 256 bits of `crypto/rand` entropy; a preimage
attack on SHA-256 of a 256-bit random input is infeasible regardless of hash speed, and no
rainbow table can cover the space, so salting and stretching add nothing. A KDF would also run
on **every API request** (unlike login, which is rare), handing an unauthenticated attacker a
CPU-amplification lever — the opposite of the H4 lesson. SHA-256 is the correct choice, not a
compromise.

### 2.3 Validation (constant-time by construction)

At startup the server loads all `api_keys` rows into an in-memory cache
(`map[id]{digest [32]byte, revoked bool, label string}`); the create/revoke handlers mutate the
cache synchronously in the same process (single-replica is the supported deployment; see §14).
Validation therefore does zero SQLite I/O:

1. Parse the `Authorization` header (§3). Failure → uniform 401.
2. Extract the id substring; look up the cache entry.
3. Compute `sha256(token)` and compare with `subtle.ConstantTimeCompare` against the stored
   digest. If the id was **not found**, compare against a package-level dummy digest instead —
   the not-found path performs the same hash and the same compare, so found-vs-not-found timing
   is equalized. (The id is non-secret by design, so even a residual map-lookup timing signal
   discloses nothing that the UI and logs don't already treat as public.)
4. If the compare fails, or the entry is revoked, return the uniform 401. Digests are
   fixed-length (32 bytes), so the compare is constant-time by construction; SHA-256 itself is
   data-independent in timing.

All failure modes — missing header, malformed header, wrong-format token, unknown id, wrong
secret, revoked key — return the identical response: `401`,
`{"error":{"code":"unauthorized","message":"A valid API key is required."}}`, plus
`WWW-Authenticate: Bearer`. No variant messages, no `error="invalid_token"` attribute: a
distinguishable response is an oracle.

### 2.4 Key material hygiene

No plaintext key, secret substring, or digest ever appears in logs, error responses, audit
events, or list endpoints. Logging and auditing always use `{id, label}`. Integration requests
are attributed to a key id in the logs by the **integration middleware emitting its own
`slog.Info` line** (`"integration request", keyId, method, path`) after validation — not by
threading a context value up to the root `requestLog` middleware, which cannot see values set
by downstream `r.WithContext` calls. The root `requestLog` stays untouched (method, path,
status, duration; it never logs headers — keep it that way). The create audit event carries
`{id, label}`, never the key.

### 2.5 lastUsedAt write coalescing

`lastUsedAt` is tracked in the in-memory cache on every authenticated request and persisted to
SQLite **at most once per 60 seconds per key**. The at-most-once bound is enforced by
**compare-and-set under the key cache's mutex**: each cache entry keeps a `lastPersisted`
marker; the middleware, holding the mutex, checks `now - lastPersisted > 60s` and *advances the
marker before releasing the mutex*; only the goroutine that won the CAS issues the SQLite write
(outside the mutex). Two concurrent requests on one key therefore produce exactly one write —
the §12.9 test depends on this rule, so it is normative, not an implementation hint. A
best-effort flush runs at graceful shutdown. The admin list endpoint merges the fresher
in-memory value, so the UI shows live data while a 1 req/s poller costs one SQLite write per
minute instead of 60.

### 2.6 Revocation

Revocation is **soft**: `revoked_at` is set, the cache entry flips to revoked synchronously
(the next request fails before any handler runs), and **the key's limiter bucket is deleted in
the same critical section as the cache flip** — revocation frees limiter state immediately, so
revoke/create churn cannot accumulate dead buckets (see §8.1 for the resulting cardinality
bound). The row is retained forever: revoked keys stay in the admin list with their
`revokedAt`, preserving the audit trail (label, creation, last use). There is no hard delete
and no un-revoke in v0.4.0; re-issuing means creating a new key. Active (unrevoked) keys are
capped at **100**; create returns `409 too_many_keys` beyond that, bounding the cache and
limiter cardinality by construction.

## 3. Authorization header parsing

Exact rules; anything not matching returns the uniform 401 of §2.3.

```
value        = scheme SP token
scheme       = %s"Bearer"        ; matched ASCII case-insensitively (RFC 7235)
SP           = single 0x20
token        = "phk_" 8lowerhex "_" 43base64url    ; total 56 chars
lowerhex     = %x30-39 / %x61-66                   ; 0-9 a-f
base64url    = ALPHA / DIGIT / "-" / "_"
```

- Exactly one `Authorization` header. Zero, or more than one, → 401.
- Optional leading/trailing OWS around the header value is trimmed (Go's header handling does
  this); anything else — extra internal spaces, tabs between scheme and token, trailing
  garbage, wrong token length or charset — → 401.
- A malformed header is `401`, not `400`. RFC 6750 permits 400 for malformed requests, but a
  distinct status would let a prober distinguish "format-valid key" from garbage; uniformity
  wins.
- No query-parameter, cookie, or request-body token transport, ever. The session cookie is
  ignored entirely on `/api/integration/v1` — a browser session grants nothing there.

## 4. Endpoints

All endpoints are GET, JSON, RFC 3339 UTC times, and share the envelope:

```json
{ "data": ..., "lastParseAt": "2026-07-10T02:00:00Z", "nextCursor": "djF8..." }
```

`data` is always present. `lastParseAt` appears on SAV-derived endpoints (`/players`,
`/players/{uid}`, `/pals`, `/guilds`) and is the store's `world_state.last_parse_at` (`null` if
no parse has completed) — the staleness signal for everything save-derived. `nextCursor`
appears only on paginated endpoints; its exact semantics are defined in §7.1.

Responses are built from **dedicated typed view structs**. The ban is on *any*
`map[string]any` producer, not just the session view functions: in particular
`store.GuildJSON` and `store.Pals` return `[]map[string]any` shaped for the session UI and
**must not** be reused on this surface — `/guilds` and the pal roster get dedicated store
queries (or a re-shape) into integration view structs, and `/metrics/current` gets a dedicated
view struct copied field-by-field from `poller.CurrentMetrics` rather than serializing the
poller type directly. The rationale is allowlist-by-construction: a field added to a shared
store map or poller struct for the session UI must never walk onto the token surface
automatically — adding a field here must require touching an integration view struct that the
redaction tests (§12.4) watch. (Note: Go's `encoding/json` sorts map keys, so determinism is
*not* the argument against maps — structs and maps both serialize deterministically; the
argument is structural redaction, and it is sufficient.)

| Route | Shape of `data` |
|---|---|
| `GET /players` | `[{uid, name, online, level, guildId, guildName, firstSeenAt, lastSeenAt, playtimeSec, captureTotal?, uniquePalsCaptured?, paldeckUnlocked?}]` — paginated; optional progression is decoded from the per-player save's typed `RecordData` and omitted, never zero-filled, when unavailable; `?online=true` supported with the exact mechanics of §7.1 |
| `GET /players/{uid}` | one player object as above plus rich save-derived `pals`, using the same Pal fields as `/pals` except owner projection. The `{uid}` segment must match `^[0-9a-fA-F-]{1,36}$` before any store call; invalid or unknown input is `404 not_found`. |
| `GET /pals` | Bulk paginated roster with identity, rare flags, placement, owner provenance, and actual per-instance `hp`, normalized `gender`, `talents: {hp, melee, shot, defense}`, `passiveSkillIds`, and `equippedSkillIds`. `placement=base` plus `baseId` is derived only from an exact WorkerDirector-container join; `baseId` joins `guilds[].bases[].id`, while raw container GUIDs remain redacted. Work suitability and species base stats deliberately remain out of the snapshot contract and are joined client-side from version-pinned Paldeck data. |
| `GET /guilds` | `[{id, name, adminUid, memberCount, members: [{uid, name}], bases: [{id, location: {x, y}, level}]}]` — not paginated (guild counts are tens, response bounded by save size; pagination here is future work if a world proves otherwise) |
| `GET /map` | `{source, gameVersion, fetchedAt, notes, layers: [{id, label, format, tileSize, minZoom, maxZoom, transform: {a,b,c,d}, bounds}]}` — dataset metadata sufficient to plot base coordinates on a client's own map. camelCase (a deliberate re-shape of the sidecar-mirroring `/api/v1/map/dataset`); the `path` field is omitted (it only serves session tile-URL construction). **No tile images** |
| `GET /server` | `{name, description, version, state, uptimeSec}` — `worldGuid` and `panelVersion` are redacted (§6). **Data source: the poller's cached last-successful `Info` snapshot** (the poller already round-trips Palworld REST on `PALHELM_METRICS_INTERVAL`; extend it to retain the last successful `Info` result if it does not already). The handler **never** makes a per-request upstream REST call — the session `serverInfo` does, and copying that here would hand token holders up to 100 keys × 60 req/min = 6,000 upstream calls/min against the process running the game. **Unreachable shape:** when no successful snapshot exists or the poller currently reports REST unreachable, respond `200` with `state: "unreachable"` and empty-string/zero remaining fields — never a 500, never an upstream error message |
| `GET /metrics/current` | `{fps, fpsAvg, frameTimeMs, players, maxPlayers, day, uptimeSec, baseCamps}` — field-for-field copy of `poller.CurrentMetrics` into a dedicated view struct (§4 struct rule; no identity content today, and the dedicated struct keeps it that way if `CurrentMetrics` grows panel-only fields) |
| `GET /events` | `[{at, kind, message}]` — bounded recent public activity (`limit` default 50, min 1, max 100). Only well-formed join/leave names, generic `Backup completed`, and the four explicitly allowlisted REST-reachability/save-drift system transitions are returned. `meta` is structurally absent; panel/config/audit events and unknown system text are discarded. The handler scans at most 500 newest store rows and does not expose a cursor or kind filter, preventing raw event categories from becoming a probing surface. |

`/world` is deliberately not exposed: `formatDrift`/`skippedProps` are operator diagnostics,
and the useful part (`lastParseAt`) is already in every SAV-derived envelope.

Unknown paths under `/api/integration/v1` with a valid token return `404 not_found` with the
standard envelope. Without a valid token, every path — valid or not — returns the uniform 401,
because auth middleware runs before routing (§1).

Integration 500s use the generic message `"The server could not complete the request."`;
internal error text goes to `slog` only. (The session API's `internal()` helper currently
echoes `err.Error()`; the integration group must not reuse it.)

## 5. Caching and response headers

- `Cache-Control: no-store` on **every** integration response, set by group middleware. A
  per-endpoint enumeration was considered and rejected: `/players`, `/players/{uid}`, `/pals`,
  and `/guilds` are identity-bearing (names, uids, presence), and blanket no-store on the
  remaining four (`/map`, `/server`, `/metrics/current`, `/events`) costs nothing while eliminating a
  class of "new endpoint forgot the header" regressions.
- `Cache-Control: no-store` on **every `/api/v1/integration-keys` response** as well. These
  routes live in the session `adminOnly` group, outside the integration group's middleware, and
  the `POST` 201 is the **only** response that ever carries a plaintext key — it must never be
  storable by a browser or intermediary cache. Mechanism: the `securityHeaders` path list
  (server.go, currently `/api/v1/auth/`, `/api/v1/config`, `/api/v1/config/raw`) **gains the
  `/api/v1/integration-keys` prefix**, so the header is set centrally like the other sensitive
  session responses rather than per-handler. This is a tested invariant (§12.10), not a
  convention.
- **CORS: none in v0.4.0 — decided, not omitted.** No `Access-Control-Allow-*` header is set
  anywhere on the token surface (or elsewhere; `server.go` sets none today). Browser-based
  dashboards on another origin therefore cannot call this API directly and **must proxy
  server-side**, keeping the bearer key out of browser-held JavaScript — which is exactly the
  theft posture §15 assumes. A permissive `Access-Control-Allow-Origin: *` on this group is
  explicitly forbidden in this release; revisit only together with scoped keys (§14).
- Root security headers apply automatically (verified, §1).
- `WWW-Authenticate: Bearer` accompanies every 401 from the group.
- `Retry-After` accompanies every 429 (§8).

## 6. Privacy and redaction policy

Posture: these tokens serve public-facing Discord bots; assume every response body ends up
pasted in a public channel. The surface is therefore **viewer-minus**: strictly a subset of
what the read-only viewer sees today, never more. Redacted fields are **absent from the
response**, not `null` — omission is enforceable with string-level assertions and never
mistaken for "temporarily unknown".

Per-field decisions against the session `playerView` (server.go):

| Field | Token surface | Justification |
|---|---|---|
| `uid` | **expose** | Save-derived GUID, the join key for `/players/{uid}`, `/pals.ownerUid`, guild members. Not a platform credential; without it the API is unusable |
| `steamId` / platform ids | **redact** | Durable real-platform identity; enables profile lookup, cross-server correlation, doxxing/harassment from a public channel. Bots key on `uid` and display `name`; no legitimate read-only use case |
| `accountName` | **redact** | Platform account name (distinct from in-game display name); same identity-correlation risk as `steamId` |
| `name` | **expose** | In-game display name — the point of the API |
| `online` | **expose** | Presence roster is the primary bot use case |
| `level` | **expose** | Game-progress stat; leaderboard use case |
| `guildId`, `guildName` | **expose** | Consistent with `/guilds` being exposed |
| `ping` | **redact** | Network telemetry hinting at geography/connection quality; zero bot value, nonzero harassment value ("laggy player X") |
| `location` (player, live) | **redact** | Real-time position of a person's character is a stalking/raid-targeting primitive on PvP servers when piped to a public channel. The viewer UI shows it inside an authenticated panel; a token response has no such containment |
| base `location` (guilds) | **expose** | Bases are persistent, communal structures already discoverable by everyone on the server; plotting them is the stated purpose of `/map`. Unlike live player position, a base location is not a real-time tracking signal |
| `firstSeenAt`, `lastSeenAt` | **expose** | Coarse presence history; "last seen" is a staple bot feature and is already implied by join/leave visibility in-game |
| `playtimeSec` | **expose** | Leaderboard staple; aggregate, non-locating |
| `banned` | **redact** | Moderation state is the operator's business; publishing it invites public shaming and re-litigating moderation in channels |
| `whitelisted` | **redact** | v0.3.0 established this is an unenforced local annotation ledger (operator notes); leaking operator bookkeeping has no reader value |
| `sessions` (player detail) | **redact** (omitted) | Fine-grained connection log; a per-person timeline is surveillance material. `lastSeenAt` covers the legitimate need |
| Pal identity, level, rare/placement fields, HP, gender, talents, passive IDs, equipped-skill IDs | **expose** | Save-derived game-progress/build-planning data; no account or infrastructure identity |
| `worldGuid` (`/server`) | **redact** | Infrastructure identifier tied to on-disk save layout and backup selection; no bot renders it, and it fingerprints the deployment |
| `panelVersion` (`/server`) | **redact** | Advertises the exact Palhelm build to anonymous-ish holders of leaked tokens; aids vulnerability targeting. Game `version` stays (players legitimately ask "what version is the server") |
| `/metrics/current` fields | **expose** (all) | Server performance/uptime telemetry; standard public status-bot content, no identity |
| redacted recent events | **expose** | Join/leave names are ordinary in-game presence, backup text is replaced with a generic state, and only four operational state transitions are allowlisted. `meta`, panel/config/audit text, file paths, actors, and unknown system messages are never serialized |

Enforcement is structural: every integration endpoint serializes a dedicated allowlist struct
whose fields are exactly the "expose" rows. There is no code path from `store.Player.SteamID`
(or `Ping`, `X`, `Y`, `Banned`, `Whitelisted`, `Raw`) into any integration view struct.
Redaction tests are string-level with proven-non-vacuous fixtures (§12).

Out of scope **entirely** (never on the token surface in raw/session form): `/config`, `/config/raw`,
`/console/*`, `/backups/*`, `/whitelist`, all auth/session endpoints including `/api/v1/events`,
`/events/stream`, `/world/parse`, map tile images (`/map-tiles/*`), paldeck icons and the icon
dataset. The raw REST JSON blob (`players.raw_json`) and pal `raw_json` never leave the store
for this surface.

## 7. Pagination and conditional requests

### 7.1 Keyset cursors

**Decision: keyset (cursor) pagination, not offset.** Offset pagination breaks under
`ReplaceWorld`: a save re-parse deletes and reinserts the `pals` table, and any concurrent
insert/delete shifts offsets, producing duplicates or gaps across pages. Keyset pagination over
an immutable unique key is immune to shifting.

- `/players`: `ORDER BY uid ASC`, page key `uid`. (`ORDER BY name`, as the session API uses, is
  not stable — names duplicate and change.)
- `/pals`: `ORDER BY instance_id ASC`, page key `instance_id`.

Both keys are save-derived GUIDs: unique, immutable per entity, and stable across re-parses of
the same world. Parameters:

- `limit`: default 100, min 1, max 500. Out-of-range or non-integer → `400 invalid_limit`.
- `cursor`: opaque; `base64url("v1|" + <key>)`. The server returns it as `nextCursor`; clients
  must treat it as opaque. Undecodable, wrong-version, or wrong-charset cursors →
  `400 invalid_cursor`. Query: `WHERE key > ? ORDER BY key ASC LIMIT ?`. An empty `cursor`
  parameter (`?cursor=`) is treated as absent — the first page — not as an undecodable value;
  this is friendlier than a 400 for an empty-but-present parameter and discloses nothing, so
  it is spec-blessed rather than an implementation quirk.
- **`nextCursor` semantics (normative):** `nextCursor` is always the key of the last row
  **returned** in `data`, and is `null` whenever the query returned **fewer than `limit`**
  rows. When the total row count is an exact multiple of `limit`, the client receives a
  non-null cursor whose next page is `{"data": [], "nextCursor": null, ...}` — an empty final
  page is a valid response, and an empty `data` with a **non-null** `nextCursor` is impossible
  by construction. Clients stop when `nextCursor` is `null`.
- **`?online=true` (players only) — exact mechanics:** online status exists only in the
  poller's in-memory map (`s.poll.Online()`), not in SQLite, so it cannot appear in the keyset
  WHERE clause as a column. The handler snapshots the current online UID set — bounded by the
  game's max-players setting (≤ 32 on a vanilla server, always small) — and passes it into the
  same query as an additional predicate: `WHERE uid IN (?,...) AND uid > ? ORDER BY uid ASC
  LIMIT ?`. The keyset predicate, ordering, and LIMIT are unchanged, so the cursor works
  identically; `nextCursor` follows the normative rule above (last *returned* row, null on a
  short page). **Zero-online short-circuit (normative):** when the snapshot is empty — the
  normal state of an idle server — the handler returns `{"data": [], "nextCursor": null, ...}`
  without querying at all; mechanically emitting `IN ()` is a SQLite syntax error and must be
  unreachable. Filtering after `LIMIT`, or loading the whole table and filtering in memory,
  are both forbidden — the first can emit an empty page with a non-null cursor, the second
  voids the bounded-query rationale. Any query value other than `true` (absence aside) →
  `400 invalid_request`.

**Consistency guarantee (documented in API.md):** each page is internally consistent —
`ReplaceWorld` swaps table contents in a single transaction and `SetMaxOpenConns(1)` serializes
readers against it, so no page observes a half-replaced table. Across pages: no row is ever
duplicated, and every row that exists for the entire pagination window is returned exactly
once. Rows created or deleted mid-pagination may be included or missed — the standard keyset
contract; the per-response `lastParseAt` lets clients detect that a re-parse landed mid-walk
and restart if they need a snapshot.

### 7.2 ETag

**Decision: weak content-hash ETag, on every integration 200 (and echoed on the resulting
304) — never on 4xx/5xx responses.** `Last-Modified` was rejected: `/players` blends
15-second live poller data with parse data, so no single honest modification time exists, and
second-granularity `If-Modified-Since` races the 5s metrics poller.

Mechanics: the handler builds the response body, computes `SHA-256(body)`, and sets
`ETag: W/"<first 32 hex chars>"`. `If-None-Match` is parsed as a comma-separated list and
compared **member-wise** using weak comparison (byte-equal opaque tags, `W/` prefix
insensitive; a bare `*` matches any current representation) — never whole-header string
equality. On a match, respond `304` with no body and the same `ETag`/`Cache-Control` headers.
Deterministic serialization is a non-issue either way (Go's `encoding/json` emits struct
fields in declaration order and sorts map keys); the typed view structs are mandated by §4/§6
for redaction, and the ETag simply inherits their stable output.

This is an **application-level** revalidation channel: `Cache-Control: no-store` (§5) forbids
cache storage, but a polling bot holds the last ETag in memory and spends one cheap request
instead of re-downloading a multi-hundred-row body. Honest cost accounting: a 304 saves
bandwidth, not query work — the server still runs the (paginated, rate-limited) query. That is
acceptable; the limiter bounds total work.

## 8. Abuse controls

### 8.1 Per-token rate limiter

A new limiter reuses the `auth.go` pattern (sliding-window timestamp buckets, expiring entries,
bounded map, fail-closed) with these parameters:

- **Key: the API key id.** Never the IP — token identity is stronger than network identity, so
  forwarded-header spoofing (H4) is structurally irrelevant here. A test still proves
  `X-Forwarded-For` has no effect on limiting.
- **Default: 60 requests/minute per key**, sliding window. Override via
  `PALHELM_INTEGRATION_RATE_LIMIT` (integer ≥ 1; invalid values fail startup — fail-closed, not
  silently default).
- **Bounds:** max 1024 limiter entries; entries expire one window after last use (pruned on
  access, as in `auth.go`). If the map is full and the key has no bucket, the request is
  **denied 429 (fail-closed)** — identical to `maxLimiterEntries` behavior in `auth.go`.
- **Global ceiling by construction:** buckets are created only for *validated* keys (§8.2),
  active keys are capped at 100 (§2.6), and **revocation deletes the key's bucket in the same
  critical section as the cache flip** (§2.6) — so limiter cardinality stays bounded by the
  active-key count (≤ 100) in steady state, including under revoke/create churn. A request
  already in flight when its key is revoked can transiently re-create the deleted bucket; that
  excess is bounded by in-flight concurrency, expires after one window, and is absorbed by the
  1024-entry map bound. (Without the revoke-time deletion, churned buckets would linger up to
  one window and the "≤ 100 by construction" claim would be false; the deletion plus the
  in-flight caveat is the honest bound.) Worst-case
  aggregate load is 100 × 60 = 6,000 req/min, far under the 1024 map bound, which remains as
  defense in depth. No separate aggregate counter is needed in v0.4.0; multi-replica shared
  state is future work (§14).

429 envelope, matching the documented error shape exactly:

```json
{ "error": { "code": "rate_limited", "message": "API key rate limit exceeded; retry later." } }
```

with header `Retry-After: <n>` — **included on every 429**, integer seconds (rounded up)
until the oldest timestamp in the window ages out. On the fail-closed map-full path there is
no bucket and therefore no oldest timestamp to compute from: `Retry-After` is the **full
window length** (60 seconds). Bots universally honor the header and it converts hammering into
polite backoff; omitting it invites tight retry loops.

### 8.2 Middleware ordering (the H4 lesson)

Exact order, no state allocation before validation:

```
parse Authorization header          → 401 on failure; allocates nothing
validate key (constant-time, §2.3)  → 401 on failure; allocates nothing, touches no DB
limiter keyed by key id             → 429 when exhausted or map fail-closed
lastUsedAt coalesced touch (§2.5)
handler
```

**Justification of limiter-after-validation vs unauthenticated flooding:** the pre-limiter
rejection path is a header parse, one in-memory map read, one SHA-256 of ≤56 bytes, and one
32-byte constant-time compare — no allocation that outlives the request, no SQLite, no limiter
bucket. Unknown or malformed keys therefore create **no state**: an attacker rotating garbage
tokens cannot grow any map (the H4 failure mode) and gains no amplification (the work they
impose is strictly less than the TLS/TCP work they already spent). A global
unauthenticated-failure counter was considered and rejected: it would itself be pre-validation
mutable state — a self-inflicted H4 — and the 401 path is already cheaper than any counter
consultation. The panel's trusted-network-edge posture (ARCHITECTURE.md) covers raw
connection-flood volumetrics, which no application counter fixes anyway.

## 9. Admin key management API

Session-authenticated, inside the existing `/api/v1` `adminOnly` group (viewer receives
`403 forbidden`, matching every other admin route — not 404; the existing behavior is the
contract). These routes are **not** on the integration router and can never be reached with a
bearer token.

| Method | Path | Request | Response |
|---|---|---|---|
| POST | `/api/v1/integration-keys` | `{"label": "discord-bot"}` — required; trimmed; 1–64 chars after trim; control characters rejected → `400 invalid_request`. Duplicate labels allowed (id disambiguates) | `201` `{"id","label","createdAt","lastUsedAt":null,"revokedAt":null,"key":"phk_..."}` — the **only** response that ever contains the plaintext key. `409 too_many_keys` at the 100-active cap |
| GET | `/api/v1/integration-keys` | — | `200` `[{"id","label","createdAt","lastUsedAt","revokedAt"}]`, newest first; `lastUsedAt` merges the in-memory value (§2.5); never contains `key` or any digest |
| DELETE | `/api/v1/integration-keys/{id}` | — | `200` with the updated record (`revokedAt` set). Idempotent: revoking an already-revoked key returns `200` with the original `revokedAt`. Unknown id → `404 not_found` |

**Why DELETE rather than POST /revoke:** removal-from-service is the resource-level operation
DELETE expresses, and it keeps the surface to three routes; that the row is soft-retained is a
storage/audit detail, documented here and in API.md, not a semantic the client manages. There
is no update/rename endpoint in v0.4.0.

Audit events (kind `panel`, via the existing `s.audit`): `"created integration key"` and
`"revoked integration key"`, meta `{"id": ..., "label": ...}` — never key material (§2.4).

### Settings UI card (for the frontend implementer)

An admin-only "Integration API" card on the Settings screen: a list of keys (label, key id,
created, last used, revoked badge), a create flow (label input → one-time key display in a
monospace copy field with an explicit "this key is shown once — store it now" warning; the
value is discarded from client state on dismiss), and a revoke action behind the existing
confirm-dialog pattern. Viewers see nothing (the card is role-gated and its API returns 403).
A one-line hint documents the curl shape:
`curl -H "Authorization: Bearer phk_..." https://host/api/integration/v1/players`.

## 10. Storage and migrations

### 10.1 Migration runner (new in this release)

`store.Open()` currently executes only `001_init.sql`; v0.4.0 needs a second migration, so the
runner arrives now. Design:

- Migrations are embedded `migrations/NNN_name.sql` files, `NNN` a zero-padded strictly
  increasing integer. `001_init.sql` is **not edited**.
- **Version tracking: the existing `kv` table's `schema_version` row.** Decided over
  `PRAGMA user_version` because `001_init.sql` already seeds `kv('schema_version','1')` — every
  v0.2/v0.3 database in the field carries it, so the runner needs no backfill or detection
  heuristics; `user_version` would be `0` on those databases and require inference. One source
  of truth already exists; use it.
- Algorithm in `Open()`, after the PRAGMAs: read current version (`0` when the `kv` table does
  not exist yet — a fresh database). **If the read version exceeds the binary's newest known
  migration, `Open()` fails closed with a clear error** ("database schema version N is newer
  than this binary supports") — a future-versioned database (e.g. v0.5 data dir under the
  v0.4 binary) must never be silently opened or partially migrated. Otherwise, for each
  embedded migration with `NNN > current`, in ascending order: **`BEGIN IMMEDIATE`** →
  **re-read `kv.schema_version` inside the transaction and skip (commit nothing) if it is
  already ≥ NNN** → execute the file → `INSERT OR REPLACE kv.schema_version = NNN` → `COMMIT`.
  Any error rolls back that migration and fails `Open()` — fail-closed; the server does not
  start on a half-migrated database. SQLite DDL is transactional, so each migration applies
  atomically.
- **Concurrency, stated honestly:** `SetMaxOpenConns(1)` is a *per-process* bound and
  guarantees nothing across processes (a second container on the same file, ops tooling).
  Cross-process safety comes from `BEGIN IMMEDIATE` acquiring SQLite's write lock up front
  (`busy_timeout=5000` waits, then errors → `Open()` fails closed) plus the in-transaction
  version re-read, which makes a concurrently-applied migration a no-op instead of a double
  application. This matters beyond 002: 002 happens to be idempotent (`CREATE TABLE IF NOT
  EXISTS`), but the runner must be correct for a future non-idempotent 003 (`ALTER TABLE ...
  ADD COLUMN`), so the re-read is a runner invariant, not an optimization.
- Fresh database: runs 001 then 002 (001's own `INSERT OR IGNORE` of version `1` is harmless;
  the runner's write supersedes it). Existing v0.3 database: version is `1`, so only 002 runs.
  Re-opening is idempotent (no pending migrations → no writes).
- **Downgrade (v0.4 database under a v0.3 binary) — supported, and now documented rather than
  accidental:** the v0.3 `Open()` executes only `001_init.sql`, which is entirely
  `IF NOT EXISTS`/`INSERT OR IGNORE`, so it opens a v0.4 database cleanly; `schema_version`
  stays `'2'` (`INSERT OR IGNORE` does not clobber it) and the `api_keys` table sits ignored.
  Keys created in v0.4 remain dormant in the database and work again after re-upgrade, which
  resumes at version 2 with no re-run. The v0.4.0 release notes must state this rollback
  contract explicitly (§13).

### 10.2 `002_api_keys.sql`

```sql
CREATE TABLE IF NOT EXISTS api_keys (
  id           TEXT PRIMARY KEY,          -- 8-char public key id
  hash         BLOB NOT NULL UNIQUE,      -- 32-byte SHA-256 of the full plaintext key
  label        TEXT NOT NULL,
  created_at   INTEGER NOT NULL,          -- unix seconds, like every other table
  last_used_at INTEGER,
  revoked_at   INTEGER
);
```

Lookups are by `id` (primary key); the `UNIQUE` on `hash` is a defensive invariant, not a query
path. No plaintext column exists by construction.

### 10.3 Upgrade test requirement

A test constructs a database by executing `001_init.sql` verbatim (simulating a live v0.3
database), inserts representative player/KV/event rows, then calls `store.Open()` on it and
asserts: `api_keys` exists and is usable; `schema_version` is `2`; the pre-existing rows are
still readable through the normal store methods; a second `Open()` is a no-op. Two further
cases: a database whose `schema_version` is set above the binary's newest migration makes
`Open()` fail with the version-too-new error (no writes performed); and the in-transaction
re-read path — a database whose version already equals a migration's `NNN` skips that
migration without re-executing it. This is the Phase 3 migration proof and a merge gate.

## 11. Error envelope

All integration and key-management errors use the documented envelope
`{"error":{"code","message"}}` with these codes: `unauthorized` (401, uniform — §2.3/§3),
`rate_limited` (429, with `Retry-After`), `not_found` (404), `invalid_limit`, `invalid_cursor`,
`invalid_request` (400), `method_not_allowed` (405, non-GET methods inside the integration
mount — §1), `too_many_keys` (409), `internal_error` (500, generic message on the integration
surface — §4). No integration error ever embeds upstream error text, file paths, SQL, or key
material.

## 12. Testing requirements (Phase 3 proofs)

Each item below is a named, adversarial proof an independent agent must land before v0.4.0 is
called done. "Test exists" is insufficient — each must state the threat, the exact test, and
why it closes the threat (the repo's review standard).

1. **Full-router scope enumeration.** Walk the real router with `chi.Walk` — not a hand-kept
   list. Assert: every route reachable under `/api/integration/v1` is GET; and for **every**
   other route in the router (session, admin, tiles, paldeck, SPA, auth, public routes like
   `/healthz` and `/api/openapi.json`, events, config, console, backups, whitelist,
   world/parse), the response status with a valid bearer token is **identical to the status
   without it** — the token must confer nothing anywhere outside its group. (Public routes
   legitimately return 200 either way; that phrasing avoids wrongly demanding 401 from them.)
   Enumerate, don't spot-check.
2. **No state before validation (H4).** Requests with missing, malformed, unknown-id,
   wrong-secret, and revoked keys: assert uniform 401 body and `WWW-Authenticate`, **zero**
   limiter entries created (inspect limiter size), zero `api_keys` writes, zero cache
   mutations. Include header variants: lowercase `bearer`, double spaces, duplicate headers,
   trailing garbage, 55/57-char tokens, uppercase hex id.
3. **Constant-time by construction.** Prove structurally: the unknown-id path executes the same
   SHA-256 + `subtle.ConstantTimeCompare` (dummy digest) as the known-id path — assert via the
   single shared validation function's coverage on both paths. Wall-clock timing assertions are
   flaky and are not the proof; construction is.
4. **Redaction, string-level, non-vacuous.** Fixture players carry sentinel values
   (`steamId: "STEAM-SENTINEL-7656..."`, `accountName`, non-zero `ping`, a location, `banned`
   and `whitelisted` set, sessions rows; fixture server info carries a sentinel `worldGuid`).
   First assert the sentinels **do** appear in the session-API responses — proving the fixture
   flows through the real pipeline (the v0.3.0 vacuous-fixture lesson) — then assert the raw
   response bytes of every integration endpoint, across all pages, contain none of them.
5. **Limiter behavior.** Per-key isolation (exhaust key A; key B unaffected), window expiry
   restores service, fail-closed denial at the 1024-entry cap, `Retry-After` present and
   plausible on 429, and spoofed `X-Forwarded-For`/`X-Real-IP` have no effect on integration
   limiting (reuse the H4 test patterns).
6. **Pagination stability.** Deterministic ordering across runs; walking pages while
   `ReplaceWorld` executes between page fetches yields no duplicate keys and no gaps among rows
   present throughout the walk; cursor tampering → 400; `limit` bounds enforced. **Online
   filter (the B2 probe):** a fixture with 250 players of which exactly 3 are online — uids
   sorting into the last keyset page — returns all 3 on the first `?online=true&limit=100`
   page with `nextCursor: null`; no response ever pairs empty `data` with a non-null
   `nextCursor`; an exact-multiple-of-limit total yields a final `{"data": [],
   "nextCursor": null}` page; **zero players online** returns `{"data": [], "nextCursor":
   null}` with status 200 and no SQL error (the §7.1 short-circuit). **Uid validation:** `GET /players/%25` (and `_`, over-length,
   non-hex input) returns 404 without a store query.
7. **Migration upgrade.** §10.3.
8. **Key lifecycle.** Create → authenticate → revoke → immediate 401 (no restart, no cache
   staleness window); plaintext appears in exactly one response ever; DB row contains a 32-byte
   digest ≠ plaintext; list responses and audit events never contain the key or digest;
   creation past 100 active keys → 409.
9. **lastUsedAt coalescing.** Two authenticated requests within 60s — including two issued
   **concurrently** (exercising the §2.5 mutex-guarded compare-and-set) — produce exactly one
   SQLite write; the admin list reflects the in-memory timestamp.
10. **Headers.** Every integration response (200, 304, 400, 401, 404, 405, 429) carries
    `Cache-Control: no-store` and the v0.3.0 security header set; 304 responses carry the
    ETag; ETag appears on 200/304 only, never on 4xx/5xx. **All three
    `/api/v1/integration-keys` responses — the 201 create response above all, since it carries
    the plaintext key — also carry `Cache-Control: no-store`** (the §5 path-list extension is
    a tested invariant).
11. **OpenAPI contract tests.** Every new path: method, status codes, and response schema
    asserted against the embedded `openapi.json` (method/status/shape, not path-presence — the
    M11 lesson).
12. **Envelope conformance.** 401/404/429/400 bodies parse as the documented error envelope.
13. **`/server` upstream isolation.** Requests to the integration `/server` cause zero calls
    on the fake Palworld REST client (poller-cache reads only); with no successful snapshot,
    the response is `200` with `state: "unreachable"` and zero-value fields, not a 5xx.

## 13. Documentation obligations

- `docs/API.md`: new "Integration API" section — token semantics, header rules, the redaction
  table (§6), pagination/ETag contract, rate limits, the CORS decision (§5), and the three
  key-management routes. It must include the transport guidance: **serve the Integration API
  over TLS at the network edge and treat keys as passwords** — bearer keys over cleartext HTTP
  are readable on-path; the panel's trusted-edge posture (ARCHITECTURE.md) is the operating
  assumption, and this sentence makes it explicit to key holders.
- Embedded `openapi.json`: all seven integration paths + three key-management paths, with
  `bearerAuth` (`type: http, scheme: bearer`) security scheme scoped to the integration paths
  only, full response schemas, and the error envelope schema referenced by 4xx responses.
  `/api/openapi.json` is served unauthenticated (server.go), so the document is public:
  schemas must contain no secrets, and any example key value must be **obviously fake and
  format-non-conforming** (e.g. `phk_00000000_EXAMPLE-NOT-A-REAL-KEY` — wrong charset/length
  on purpose, so it can never validate and never trains a scanner to ignore the prefix).
- `docs/ARCHITECTURE.md`: auth section gains the bearer principal and the two-namespace
  diagram; storage section gains the migration runner.
- `README.md`: short "Integration API" section with one curl example (same fake-key rule).
- `docs/releases/v0.4.0.md`: token semantics and storage, the exact redaction policy, rate
  limits, the TLS/treat-keys-as-passwords guidance, migration notes — v0.3 databases upgrade
  in place; back up the data dir first, as always; **rollback to the v0.3 binary is supported**
  (the v0.4 schema opens cleanly under v0.3, keys stay dormant in `api_keys`, re-upgrading
  resumes at version 2 — §10.1) — and known limitations (§14).

## 14. Future work (deferred, listed only)

Webhooks (Discord join/leave push), SSE for bearer tokens, write scopes / scoped keys, the
Discord integration itself, `game-data` endpoint consumption when Palworld ships it,
multi-replica shared limiter and key-cache state (v0.4.0 is single-process; the in-memory
cache and limiter are process-local by design), guild pagination, per-key rate-limit overrides,
key expiry dates, `X-RateLimit-*` response headers, and CORS for browser dashboards (revisit
only together with scoped keys — §5 forbids it in this release).

## 15. Threat model

| Threat | Design response |
|---|---|
| **Token theft** (leaked bot config, pasted in channel) | Blast radius is capped by construction: read-only GET surface, viewer-minus redaction (§6) — a stolen key yields no platform IDs, no live positions, no moderation state, no config/console/backup access, and no mutation. `phk_` prefix aids secret scanners. Recovery is one admin click: soft revoke is immediate (§2.6), and `lastUsedAt` shows whether a stolen key was used |
| **Key enumeration** | Secrets carry 256 bits of entropy — online guessing is absurd even ignoring limits. Unknown ids and wrong secrets return the byte-identical 401 as malformed input (§2.3); auth precedes routing, so path existence isn't probeable either (§1). Key ids are non-secret by definition, so id discovery yields nothing |
| **Timing oracles** | Fixed-length digest comparison via `subtle.ConstantTimeCompare`; dummy compare on unknown id equalizes the not-found path; SHA-256 is data-independent; no early exits between parse and verdict (§2.3). Proven by construction, tested structurally (§12.3) |
| **Scope creep** (bearer reaching session/admin routes) | Separate mounted router containing only GET handlers; distinct principal type never stored under the session context key; key management lives only in the session `adminOnly` group. Enforced by the full-router enumeration test, not by convention (§1, §12.1) |
| **Log leakage** | Plaintext exists only in the create response; logs/audit/list carry `{id, label}` only; request logging never touches the `Authorization` header; integration 500s are generic (§2.4, §4). Tested at string level (§12.8) |
| **Redaction gaps** | Allowlist typed view structs with no code path from sensitive store fields; absent-not-null policy; sentinel-fixture string assertions proven non-vacuous against the session API first (§6, §12.4) |
| **Limiter-state exhaustion** (H4 class) | No state of any kind allocated before constant-time validation; buckets exist only for valid keys; active keys capped at 100 and revocation deletes the key's bucket synchronously, so cardinality tracks active keys even under revoke/create churn; map bounded at 1024 (defense in depth) and fail-closed; entries expire (§2.6, §8). Spoofed forwarded headers are irrelevant because limiting keys on token identity, not IP |
| **Upstream amplification** (token → game-server load) | `/server` reads the poller's cached Info snapshot; no integration handler ever makes a per-request upstream REST or RCON call, so a key holder's ceiling on game-server load is zero regardless of request rate (§4, §12.13) |
| **On-path key capture** (cleartext transport) | Out of the panel's hands beyond documentation: the trusted-edge posture assumes TLS termination at the edge; API.md and the release notes explicitly instruct operators to serve the API over TLS and treat keys as passwords (§13). No transport downgrade the panel can detect is silently accepted as "secure" (cookies already gate on `SecureCookies`/forwarded proto; bearer auth carries no equivalent flag by design) |
| **SQLite write amplification via polling** | `lastUsedAt` coalesced to ≤1 write/min/key via mutex-guarded compare-and-set (§2.5); reads are paginated and rate-limited; 304s cut response bandwidth (§7.2) |
| **Half-applied or double-applied schema upgrade** | `BEGIN IMMEDIATE` per migration, in-transaction version re-read, version write inside the transaction, `Open()` fails closed on error and on newer-than-known versions; cross-process races resolve via SQLite's write lock + busy_timeout, not per-process pool limits; upgrade/downgrade tests from the exact v0.3 schema (§10) |
