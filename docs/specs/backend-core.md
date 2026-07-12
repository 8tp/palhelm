# Spec: backend core server

Build the Palhelm HTTP server in `backend/` (module `github.com/palhelm/palhelm`, Go 1.26).
The API contract is `docs/API.md` — implement everything there EXCEPT the Backups and Config
sections (a later task; stub them returning 501 with `{"error":{"code":"not_implemented"...}}`).
Architecture context: `docs/ARCHITECTURE.md`.

## Entry point
`cmd/palhelm/main.go` with subcommands: `serve` (default) and `parse <file.sav>` (reuse savdump logic).
All config via env (define in `internal/config`, one struct, documented defaults):

- `PALHELM_ADDR` (default `:8080`)
- `PALHELM_DATA_DIR` (default `./data`) — SQLite DB, oodle lib, later backups
- `PALHELM_ADMIN_PASSWORD` (required for serve), `PALHELM_VIEWER_PASSWORD` (optional; unset = no viewer role)
- `PALHELM_SESSION_SECRET` (optional; generated & persisted into DATA_DIR if unset)
- `PALWORLD_REST_URL` (e.g. `http://palworld:8212`), `PALWORLD_REST_USER` (default `admin`), `PALWORLD_ADMIN_PASSWORD` (basic-auth password, also RCON password)
- `PALWORLD_RCON_ADDR` (e.g. `palworld:25575`)
- `PALWORLD_SAVE_DIR` (e.g. `/game/Saved` — the mounted Saved/ dir; world dir discovered under `SaveGames/0/<GUID>`, pick the GUID from REST /info worldguid when available, else newest dir)
- `PALHELM_METRICS_INTERVAL` (default `5s`), `PALHELM_PLAYERS_INTERVAL` (default `15s`), `PALHELM_SAVE_SYNC_INTERVAL` (default `10m`)

## Packages
- `internal/palworld`: typed client for the official REST API (`/v1/api/info|players|settings|metrics|announce|kick|ban|unban|save|shutdown|stop`), basic auth, 5s timeout, error taxonomy (unreachable vs 401 vs 4xx). Also a minimal Source-RCON client (handshake, auth, exec; handle multi-packet responses; vanilla Palworld RCON quirks: no UTF-16, short responses).
- `internal/store`: SQLite via `modernc.org/sqlite`. Tables: `metrics` (ts, fps, frametime_ms, players, raw 5s samples; prune >24h), `metrics_rollup` (1-min avg/min/max, prune >30d), `players` (uid PK, steam_id, names, first/last seen, playtime_sec, banned, whitelisted, level, guild fields, location, raw json), `sessions` (player uid, join_at, leave_at), `events` (at, kind, message, meta json), `saved_commands`, `console_log`, `kv` (schema version, session secret). WAL mode. Migrations embedded.
- `internal/poller`: metrics poller (REST /metrics → store, in-memory ring for /metrics/current), players poller (REST /players → diff against previous → join/leave events + session rows + player upsert), savesync poller (parse Level.sav via internal/sav → upsert players/guilds/pals/bases; skip cleanly if save dir absent; record parse stats + formatDrift flag if any decoder failures). Pollers tolerate the game server being down: mark health degraded, keep retrying, emit one "system" event on state change (not every tick).
- `internal/server`: chi router, all `/api/v1` routes from docs/API.md (minus backups/config), auth middleware (JWT HS256 in HttpOnly SameSite=Lax cookie `palhelm_session`, 7d expiry; login rate-limited 5/min/IP), role gating (mutations = admin), SSE `/events/stream` (fan-out hub fed by pollers; heartbeat every 25s), request logging, panic recovery, OpenAPI 3.1 JSON generated as a static embedded file (hand-written `openapi.json` kept in sync is acceptable), `/healthz`.
- Graceful-shutdown orchestrator for `/server/shutdown` with `countdown:true`: staged announces at T-10m/5m/1m/30s/10s (skip stages already past), then REST shutdown(waittime=10). Cancellable. State machine exposed in `/server` response (`state: "running"|"countdown"|"stopping"|"unreachable"`).
- The frontend is NOT part of this task; `/` serves a placeholder page with the wordmark "palhelm" and a link to /api/openapi.json. Leave a clean seam: `internal/server/spa.go` with an `embed.FS` hook where `frontend/dist` will mount.

## Console endpoints
`/console/exec` runs via RCON, logs to `console_log` with the acting user. `/console/log` returns history. Saved commands CRUD in store. Audit every admin mutation (kick/ban/shutdown/exec/announce) as an `event` row too.

## Player identity
REST /players gives `playerId`/`userId` (e.g. `steam_7656...`); the sav parser gives player UIDs (guid). Correlate: the player UID (guid hex, e.g. 84C20A31-0000-...) — REST `playerId` is the same UID. Normalize: store uid as lowercase guid string without dashes prefix matching; keep both userId (steam_...) and uid. Kick/ban/unban take the REST `userId` — store it and use it for moderation calls.

## Testing
- Unit tests: RCON packet encode/decode; auth middleware (role matrix, expired token); metrics rollup + prune; player-diff join/leave logic; shutdown countdown stage math (fake clock).
- An integration-style test with `httptest` faking the palworld REST API (info/metrics/players) exercising: login → metrics current/history → players list → kick (asserts the fake got the call) → events.
- `go vet ./...` and `go test ./...` green. No network in tests. Keep internal/sav untouched (its tests must stay green).

## Ground rules
Work only in `backend/`. No git commits. Dependencies: chi v5, modernc.org/sqlite, golang-jwt/jwt/v5; nothing else heavy without a comment justifying it. Log with `log/slog`. Keep handlers thin; logic in services. Every exported symbol documented. Wire everything in main so `PALHELM_ADMIN_PASSWORD=x PALWORLD_REST_URL=... go run ./cmd/palhelm serve` works.

Finish with: summary, test output, the exact env vars needed to run against a real server, and any contract deviations from docs/API.md (update docs/API.md inline if you must deviate, and say so).
