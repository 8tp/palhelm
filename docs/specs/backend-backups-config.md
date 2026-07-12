# Spec: backups engine + config editor (backend, final chunk)

Implement the `/backups` and `/config` sections of docs/API.md (replace the 501 stubs).
Module: `backend/`. Read docs/ARCHITECTURE.md ("Config editing and the env-var trap", backup
engine bullet) first.

## Backups (`internal/backup`)

- Store: tar.gz archives in `<PALHELM_DATA_DIR>/backups/`, named
  `world-YYYY-MM-DD-HHMM.tar.gz`, containing `SaveGames/0/<GUID>/**` relative to the save dir
  (plus `<GUID>` recorded in a small `palhelm-backup.json` manifest inside: createdAt, trigger,
  worldGuid, panel version, in-game day if known from last parse).
- Index in SQLite (`backups` table: id, file, created_at, size_bytes, trigger, world_day).
  Reconcile with the directory on startup (files added/removed out-of-band → imported/pruned;
  imported files get trigger "imported").
- Scheduler: interval + retention from `backups_schedule` kv (API GET/PUT
  `/backups/schedule`); defaults enabled, every 240 min, keep 30 days. Runs REST `/save`
  first (best-effort, logged if unavailable) and waits ~2s for the save to flush, then
  archives. Retention prune after each run. Emits `backup` events.
- `POST /backups` → manual trigger (same flow, trigger "manual"). 409 if one is running.
- `GET /backups/{id}/contents` → tar listing (path, size, modTime), capped at 10k entries.
- `POST /backups/{id}/restore/dry-run` → extract-to-temp + walk both trees → change list
  (add/modify/delete vs the live save dir, size before/after; compare by size+mtime, and
  SHA-256 when sizes match but mtimes differ). Always `requiresStop: true`.
- `POST /backups/{id}/restore` body `{"confirm":"RESTORE"}`:
  1. Refuse (409, clear message) if the game server is reachable via REST (it must be stopped;
     checking reachability IS the check).
  2. Take a `pre-restore` backup of the current save dir first.
  3. Extract to `<dataDir>/restore-tmp-<rand>`, then atomically swap: move current world dir
     aside, move restored dir in, remove the aside copy only after success (keep it on failure).
  4. Emit event; return summary. Any failure must leave the original save intact.
- Download: stream with Content-Disposition; support HEAD.
- Path safety: reject tar entries with `..`/absolute paths on extract (zip-slip).

## Config editor (`internal/gameconfig`)

- Inputs: `PALHELM_COMPOSE_FILE` (path to the operator's docker-compose.yml, optional),
  `PALHELM_GAME_SERVICE` (default `palworld`), `PALHELM_DOCKER_CONTROL` (default false),
  plus read-only access to `<PALWORLD_SAVE_DIR>/Config/LinuxServer/PalWorldSettings.ini`.
- Settings catalog: a static table in Go mapping the thijsvanloef image's env vars ↔ ini keys
  ↔ types/groups/defaults. Cover at least: SERVER_NAME, SERVER_DESCRIPTION, SERVER_PASSWORD,
  ADMIN_PASSWORD (masked: value never returned in GET — `"•••"` placeholder, settable),
  PLAYERS, PORT, PUBLIC_IP, PUBLIC_PORT, MULTITHREADING, COMMUNITY_SERVER, and the common
  gameplay rates the image passes through (EXP_RATE, PAL_CAPTURE_RATE, DAY_TIME_SPEED_RATE,
  NIGHT_TIME_SPEED_RATE, PAL_SPAWN_NUM_RATE, DEATH_PENALTY, DIFFICULTY). Group: general,
  gameplay, network, panel-managed (RCON_ENABLED, RCON_PORT, REST-related — read-only via API).
- `GET /config`: merge three sources — compose env (desired), live REST `/settings`
  (effective), ini file (fallback effective when REST down). `pending` = desired ≠ effective.
  If no compose file is configured, `source:"ini"` and the PUT endpoint returns 409 with a
  clear message (read-only mode).
- `PUT /config`: edits ONLY the `environment:` mapping of the configured service in the
  compose file. Must preserve file comments, ordering, quoting style of untouched lines —
  do string/line-level surgery on the env block (parse positions with yaml.v3 node API or
  line-scan the block; do NOT re-marshal the whole document). Validate values by catalog type.
  Write atomically (tmp + rename). Keep one timestamped `.palhelm.bak` of the original per day.
- `GET /config/raw`: the ini text.
- `POST /config/apply`: intentionally disabled as of v0.3.0 — always 501 with
  `{error: {code: "docker_apply_disabled", manualCommand: "docker compose up -d palworld"}}`.
  Container-side Compose cannot safely preserve host project identity or relative host bind
  paths (review finding H9), so Palhelm never executes docker; the operator runs the manual
  command from the host directory containing the compose file. No docker.sock mount is
  needed or recommended.
- Audit events for every write/apply.

## Tests
- Backup roundtrip on a temp save dir fixture (create → contents → dry-run vs modified live
  dir → restore with REST-unreachable faked → file trees identical; original preserved on
  injected extract failure). Zip-slip rejection test.
- Compose surgery: table tests on a fixture compose file (edit value, add missing key,
  quoting/comment preservation asserted byte-for-byte outside the touched lines; the live
  file's shape at /path/to/your/docker-compose.yml is the reference shape — copy it
  into testdata with secrets replaced).
- Catalog: env↔ini mapping spot checks.

Ground rules: work only in backend/ (do not touch internal/sav or third_party — a hardening
task may be running there in parallel; if you must touch shared files like go.mod, make
minimal additive edits). No git. `go test ./... -count=1` and `go vet ./...` green.
