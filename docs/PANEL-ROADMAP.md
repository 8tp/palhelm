# Palhelm Panel Roadmap

Last updated: 2026-07-14

## Direction

Palhelm already has the difficult safety foundation: read-only save parsing,
role-gated server actions, guarded backup/restore, compose-aware configuration,
live performance/map views, and a redacted Integration API. The next releases
should improve operational truth, history, observability, and standalone-product
maturity before adding broader control over the host or game process.

## Phase 0 — release truth and reproducibility

- [ ] Split the mixed panel, bot, parser, migrations, and documentation work into
  reviewed commits and tagged releases.
- [x] Use one backend/build-provided panel version everywhere; remove hardcoded
  login and navigation versions.
- [ ] Publish immutable image tags/digests and document migration/rollback evidence.
- [ ] Add release smoke checks for auth, health, players, backups, config
  capabilities, OpenAPI, and the Integration API.

Exit: an operator can identify, rebuild, deploy, and roll back the exact running
panel without relying on an untracked working tree or a mutable `latest` tag.

## Phase 1 — events, audit, and real-time state

- [x] Add a real `/events` route; remove the dashboard's dead `#/events` link.
- [x] Filter and page join/leave, backup, system, panel, and config events.
- [ ] Distinguish player activity from operator/audit actions and health incidents.
- [x] Use the existing SSE stream to invalidate query caches for metrics, players,
  events, backups, health, and save state, retaining polling as fallback.
- [ ] Add export and retention controls only after sensitive event fields have an
  explicit role/redaction policy.

Exit: current state updates promptly and administrators can reconstruct what
happened without reading container logs.

## Phase 2 — activity, Pals, and guilds

- [ ] Add current-session duration plus daily/weekly/monthly player activity.
- [ ] Add concurrency, peak-hours, new/returning-player, and guild-activity views
  with explicit tracking coverage.
- [ ] Add a server-wide Pal explorer with search, owner provenance, party/box/base
  placement, Alpha/Lucky/Boss filters, and current records.
- [ ] Add Paldeck/capture progression and dedicated guild detail pages linking
  members, bases, Pals, activity, and map locations.

Exit: the panel exposes the useful save-derived information already available to
the Discord bot instead of limiting it to player detail dialogs.

## Map accuracy and exploration

- [x] Correct Palworld's flipped save-coordinate axes and use the exact game-map
  conversion for coordinate readouts.
- [x] Preserve THGL's authoritative transform offsets in the tile-fetch workflow;
  do not derive offsets from bounds.
- [x] Remove unverified pre-1.0 fast-travel and boss-tower placeholders.
- [ ] Roll out the corrected frontend and dataset metadata together after taking a
  rollback point; do not mutate the currently served dataset in place.
- [ ] Import versioned, licensed 1.0 points of interest with provenance, validation,
  and graceful version mismatch handling.
- [ ] Add focus-current-player, fit-online-players/bases, marker clustering, map
  search, shareable coordinates, and mobile controls.
- [ ] Add automated landmark fixtures across Palpagos and the World Tree so axis,
  offset, layer-boundary, and inverse-coordinate regressions fail in CI.

### Palworld 1.0 live game-data track (development branch; not deployed)

- [x] Document the official `/v1/api/game-data` schema, capability uncertainty,
  privacy boundary, and the distinction between transient actors and spawn data.
- [x] Add an opt-in, bounded, memory-only client and one shared cached poller with
  explicit ready/stale/unsupported/unauthorized states and transient backoff.
- [x] Add an authenticated panel snapshot projection and an aggregate-only,
  redacted Integration API summary; IPs and platform user IDs are discarded at
  the upstream decode boundary.
- [x] Let the map reconcile sanitized coordinates from a complete `ready`
  snapshot onto the authoritative REST roster by exact unique active-player
  name, while retaining REST/save fallback for partial, stale, or ambiguous data.
- [x] Expose session-only rollout diagnostics for request duration, accepted
  actor count, bounded failure category, truncation, and retry schedule without
  extending the public Integration summary.
- [ ] Verify endpoint support and coordinate semantics against a disposable or
  explicitly approved server session before enabling it in production.
- [ ] Join `BaseCampPal.InstanceID` to save-derived Pals and WorkerDirector base
  IDs, then add clustered live worker status to base/map views.
- [ ] Add deterministic bot tools for aggregate world health and exact-linked
  base-worker status; keep raw locations and action strings out of Discord/AI.
- [ ] Import a separately licensed, versioned 1.0 spawn/POI dataset. Never treat
  loaded `WildPal` sightings as proof of a spawn zone, schedule, or catch rate.

Exit: live markers, cursor coordinates, and points of interest agree with the
in-game map across every installed world layer.

## Phase 3 — diagnostics and integrations

- [ ] Add an admin diagnostics page for poller lag, parser duration/drift, last
  successful REST/RCON/save operations, backup freshness, database/schema size,
  disk pressure, asset datasets, and background operations.
- [ ] Add a Prometheus exporter for server and Palhelm health metrics.
- [ ] Add generic/Discord webhooks for bounded allowlisted backup, outage,
  join/leave, and configuration events.
- [ ] Add scoped Integration API keys and optional read-only event streaming.

Exit: external monitoring and incident diagnosis do not require scraping the UI
or granting an integration administrator credentials.

## Phase 4 — backup and configuration maturity

- [x] Exclude the game wrapper's direct per-world `backup/` subtree from Palhelm
  archives so scheduled backups do not recursively embed redundant restore
  points; retain active world, player, option, and configuration files.
- [ ] Add optional encrypted offsite replication through an operator-configured
  S3-compatible/restic/rclone adapter.
- [ ] Verify manifests/checksums and support scheduled restore drills into a
  temporary location without touching the live world.
- [ ] Add backup storage budgets, replication state, retries, and alerts.
- [ ] Add configuration history, diffs, rollback, named profiles, and maintenance
  planning with a required pre-change backup.
- [ ] Keep host-side apply explicit unless a narrowly scoped, independently
  authenticated host helper can prove start/restart capability and outcome.

Exit: a host or data-volume failure is survivable, and configuration changes are
reviewable and reversible.

## Phase 5 — identity, distribution, and scale

- [ ] Add optional named local users and OIDC/passkey/TOTP authentication.
- [ ] Add granular roles such as operator, moderator, backup manager, and viewer,
  with per-user audit attribution and session revocation.
- [ ] Publish signed GHCR images, release notes, update availability, and supported
  upgrade paths.
- [ ] Add Playwright coverage for login, roles, moderation, backup/restore,
  configuration conflicts, keyboard access, and responsive layouts.
- [ ] Route-split the frontend bundle; the current production build emits a
  roughly 611 kB minified entry chunk before gzip.
- [ ] Consider multi-server/multi-world support only after the single-server
  deployment and migration lifecycle is reproducible.

## Explicit deferrals

- No Docker socket mount or generic host shell.
- No automatic restart claims without verified external start capability.
- No speculative save editing, item/Pals injection, or anti-cheat claims.
- No claim that local player notes enforce a server whitelist.
- No automatic mod installation; begin with read-only inventory and compatibility.
- No guessed map coordinates or save-format fields.

## Operational guardrails

- Never restart or redeploy the Palworld game server as part of panel development.
- Do not deploy panel work without an explicit operator request and rollback point.
- Keep backend tests, frontend typecheck/build/lint/tests, OpenAPI coverage, and
  migration audits green.
- Treat backup archives, configuration, events, save names, and external metadata
  as untrusted input.
- Preserve viewer redaction and role-gate every mutation server-side.
