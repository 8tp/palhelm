# Palhelm Discord Bot Roadmap

Last updated: 2026-07-15

## Product direction

Palhelm is now a broad Discord companion rather than a small command add-on. The
bot has 32 registered slash commands, a shared snapshot/history layer, scheduled
social features, Palworld 1.0 knowledge and breeding tools, and an optional
read-only AI guide. The next releases should consolidate reliability, accuracy,
discoverability, and deeper workflows before adding more standalone commands.

Bot integrations should use Palhelm's public, redacted Integration API wherever
possible. Existing session-API use is limited to administration, notifications,
safe event summaries, and binary assets. No bot feature may restart, redeploy, or
otherwise remediate the panel or game server automatically.

## Shipped foundation

- One coalesced snapshot poller for `/players`, `/pals`, `/guilds`, `/server`, and
  `/metrics/current`, with a last-good snapshot and rate-limit-friendly consumers.
- Restart-safe observations, milestones, goals, weekly digest, trends, records,
  health alerts, and bot presence.
- Illustrated milestone cards with cached Steam avatars, Paldeck icons, level/
  playtime/record badges, generated original background art, and text-only
  fallback when a binary asset is unavailable.
- Ownership provenance and capture-progression fields, including honest
  `owner unavailable` behavior when the public data cannot establish ownership.
- Canonical 306-Pal collection tracking, rich `/dex`, deterministic breeding and
  worker tools, shortest owned-roster breeding paths, and combat/base suggestions.
- Pinned and disk-cached Palworld 1.0 mechanics with source provenance and
  last-good offline behavior.
- Optional `/ask` assistant with bounded OpenRouter calls, deterministic public
  snapshot tools, pinned knowledge tools, and read-only Palworld-scoped SearXNG
  search for general knowledge that is absent from the pinned dataset.
- Separate activity and milestone channel support, conservative drift handling,
  and suppressed user-facing notices for known save-format drift when configured.
- A categorized ephemeral `/help` directory.

The current command surface is:

`/status`, `/players`, `/player`, `/guilds`, `/metrics`, `/map`, `/pals`, `/box`,
`/leaderboard`, `/compare`, `/whohas`, `/records`, `/collection`, `/dex`, `/breed`,
`/breedpath`, `/workers`, `/team`, `/rare`, `/goal`, `/progress`, `/trends`,
`/history`, `/ask`, `/help`, `/diagnostics`, `/profile`, `/profileadmin`, `/pal`,
`/backup`, `/backups`, and `/announce`.

## Prioritized implementation plan

### Phase 0 — release hygiene and contract safety

Goal: make the current feature set reproducible and safe to evolve.

- [ ] Split panel contracts/migrations, bot changes, knowledge data, and docs into
  deliberate versioned commits and releases.
- [x] Document a bot and panel rollback procedure; do not couple bot rollout to a
  game-server restart.
- [x] Add a pre-panel-deploy database backup and migration verification checklist.
- [x] Add public Integration API schema/fixture tests for optional ownership and
  capture-progression fields.
- [x] Add post-deploy, read-only smoke checks for public endpoints and bot startup.
- [ ] Keep the unresolved ambiguous guild-tail parser fixture flagged until it can
  be decoded from evidence; never guess or hide a real data-integrity failure.

Exit criteria: a clean tagged baseline can be rebuilt, tested, deployed, and
rolled back without touching the running Palworld process.

### Phase 1 — discoverability and diagnostics

Goal: make the existing command surface easy to find and support.

- [x] Move command category/help metadata beside each command and generate `/help`
  from the registry rather than maintaining a second list manually.
- [x] Add a test proving every registered command appears exactly once in `/help`.
- [x] Validate the README command table against the same command registry in tests.
- [x] Add an admin-only `/diagnostics` view containing only safe cached state:
  snapshot age/population, history coverage, knowledge version/source count,
  SearXNG and AI availability, last backup, pending deliveries, and automation
  configuration. Recent latency/error rollups and exact next-digest time remain
  follow-up observability work.
- [x] Add safe end-to-end AI request timing and model/tool-call counts without
  logging questions, provider bodies, credentials, or `.env` data.
- [x] Keep diagnostics in safe logs and `/diagnostics`, not in user replies:
  alias save/player/guild/base/instance IDs before model synthesis, sanitize raw
  IDs from final prose, retain deterministic sources, and use a concise
  `AI Generated` footer.
- [x] Add separate provider/thinking, deterministic-tool, and web-search stage
  timings without logging prompts or tool payloads.
- [x] Keep admin authorization on the configured `ADMIN_ROLE_ID` runtime gate.
  Do not apply Discord's built-in Administrator default permission, which would
  hide commands from trusted members who hold the configured custom role.

Exit criteria: users can discover every command and an administrator can diagnose
staleness, knowledge, search, or provider failures without reading sensitive logs.

### Phase 2 — dependable general Palworld knowledge

Goal: answer ordinary questions quickly, cheaply, and with visible provenance.

- [x] Make the OpenRouter request deadline configurable with a bounded
  `AI_TIMEOUT_MS` instead of relying on a hard-coded value.
- [x] Add a bounded disk cache for SearXNG results; normalized-query coalescing,
  bounded in-memory TTL caching, and URL deduplication are shipped.
- [x] Filter unsafe/duplicate URLs and rank official, wiki.gg, and established
  Palworld sources ahead of generic search results while retaining source links.
- [x] Label web-derived facts as potentially version-sensitive in the reply.
- [x] Add a small versioned, licensed/attributed item-material-recipe-technology
  corpus for common questions such as meteorite uses, with web search as fallback.
- [x] Add a dependency-free local section index and a polite, resumable MediaWiki
  API importer with revision/license/source metadata and atomic last-good writes.
- [x] Complete the initial text-only ingestion after a bounded API smoke test;
  audit excluded namespaces/templates and corpus size before enabling it live.
- [x] Add official 1.0 and 1.0.1 patch notes as separately attributed, version-explicit
  documents and tag wiki claims only when the source itself establishes a version.
- [x] Render deterministic bot-built citations separately from model prose so a
  provider cannot omit or truncate the provenance returned by retrieval tools.
- [ ] Evaluate `x-ai/grok-4.5` against a fixed question set as an opt-in synthesis
  model; keep retrieval local and do not spend its context window on whole pages.
- [x] Build a 30–50 question retrieval benchmark from representative, sanitized
  `/ask` questions and measure whether the relevant corpus section appears in the
  top results before adding more retrieval infrastructure.
- [x] Evaluate whether embeddings are justified before adding one. The current
  9,974-section BM25 corpus passed 30/30 representative local retrieval cases, so
  a paid embedding API, external vector database, and local embedding dependency
  are intentionally not added yet. Revisit only when benchmark misses appear.
- [ ] If lexical retrieval later misses meaningfully phrased questions, add a small
  local embedding model and combine vector similarity with exact-name/BM25
  ranking. Keep embeddings optional, generated offline, dependency-light, and
  backed by the same attributed source sections; do not introduce a paid embedding
  API or external vector database for the current corpus size.
- [x] Add one retry for retryable non-timeout/non-rate-limit provider errors,
  while preserving a single overall interaction deadline and daily limits.
- [x] Add tests for slow search, stale cache, malformed results, provider timeout,
  prompt-injection-shaped snippets, and graceful offline answers.

Exit criteria: common Palworld knowledge does not depend on a fresh web request,
and every web-derived answer remains read-only, bounded, and cited.

### Phase 3 — interactive command UX

Goal: deepen the best workflows without multiplying commands.

- [x] Add requester-only `/dex` section navigation for overview, stats/work,
  combat/skills, and breeding/data coverage.
- [x] Add an explicitly web-backed drops/locations section with cached citations,
  plus a polite local CC BY-SA Cargo encounter cache for exact habitats and map
  coordinates.
- [x] Add requester-only previous/next controls, boundaries, expiry, and clear
  ownership scope to breeding results.
- [x] Support button-based Pal box page navigation.
- [x] Add requester-only `/history` pagination, type filters, and a strict public
  safe-event projection with generic backup text and allowlisted system events.
- [x] Link the first missing `/collection` entry into exact `/dex` and `/breed`
  follow-up actions.
- [x] Add record-category navigation and bounded, restart-safe historical
  record-holder changes.
- [ ] Persist component state minimally after a bot restart. Foreign interactions,
  normal expiry, and post-restart stale controls now receive explicit clean
  rejection instead of timing out; full view reconstruction remains follow-up.

Exit criteria: the most common exploration flows can be completed from one command
reply without repeatedly retyping names and options.

### Phase 4 — practical breeding and team planning

Goal: move from mathematically valid suggestions to actionable player plans.

- [ ] Extend breeding paths beyond fewest generations: owned count, known gender,
  incubation/attempt cost, scarce-parent protection, and alternative routes.
- [x] Allow a desired passive/trait target when the public API exposes sufficient
  safe data; explicitly report when inheritance feasibility is unknown.
- [x] Save a selected breeding plan as a player-scoped `/goal` and resume it from current roster
  changes.
- [ ] Improve combat suggestions only when safe public fields support them (IVs or
  talents, passives, active moves, and elemental matchups). Keep the existing
  heuristic visibly labeled until then.
- [x] Add focused tests for gender feasibility, duplicate parents, boss/variant
  normalization, raw save identifiers, and unavailable ownership.

Exit criteria: recommendations explain their assumptions and never present a
base-stat heuristic as a fully optimized build.

### Phase 5 — exact history, records, and health

Goal: turn observations into trustworthy long-term server stories.

- [x] Prefer lifetime capture counters and exact instance provenance over roster deltas;
  do not count ownership transfers as catches or gains.
- [x] Persist globally observed Pal instance IDs in goal state so transfers,
  temporary disappearance/reappearance, and newly resolved owners cannot be
  mistaken for a new acquisition.
- [x] Durably record holder changes for highest player level, longest playtime,
  and highest-level Pal with observed-since coverage/confidence labels and digest
  integration. Additional first-achievement categories can build on this state.
- [x] Add compact weekly recap cards for new species, level milestones, captures,
  rare finds, record changes, playtime, backups, and sampled health.
- [x] Add per-milestone visual cards for new species, first Alpha/Lucky finds,
  player levels, playtime badges, and observed records. Keep weekly multi-stat
  recap composition as the remaining item above.
- [x] Persist health-alert hysteresis and cooldown state across bot restarts.
- [x] Extend bounded durable history samples with FPS, save freshness, backup age,
  and uptime, and expose safe coverage/summary data through `/diagnostics`.
- [x] Add a strictly redacted, bounded Integration API event-history contract,
  client support, OpenAPI/spec coverage, and adversarial leakage tests.
- [x] Cut `/history` over from its compatible sanitized Session fallback after the
  new panel contract is deliberately deployed and read-only smoke-tested.

Exit criteria: historical claims distinguish exact facts from observations, and
bot restarts neither duplicate alerts nor lose active incident state.

### Phase 6 — Game Data activity and attributed exploration

Goal: use the official live Game Data surface without exposing sensitive actor
or location data through public bot replies.

- [x] Add a redacted Integration `/world/workers` contract joined by exact save
  instance identity, with guild-base grouping, owner provenance, health/activity,
  OpenAPI coverage, and one shared bot snapshot consumer.
- [x] Add `get_live_base_workers` for exact current-base questions and a
  deterministic timeout fallback so provider failures do not discard live data.
- [x] Add operator-facing panel diagnostics and bounded 30-day aggregate activity
  samples without raw actor-location history.
- [x] Import the Palworld Wiki `LocationEntity` Cargo table politely into a local,
  attributed cache and expose it through `/dex`, `/map pal:`, and
  `get_pal_locations`.
- [ ] Add general POI overlays only after a current, redistributable 1.0 POI schema
  is identified. Do not project wiki map coordinates onto tile pixels until the
  coordinate transform is verified with known landmarks.

## Features to defer

- A virtual currency/economy or other engagement mechanics without a real server
  need.
- Autonomous backup, restart, moderation, or remediation decisions made by AI.
- Unrestricted arbitrary-web tools or answers that omit sources.
- Additional novelty slash commands before help, diagnostics, and interactive UX
  are complete.
- Parser assumptions added solely to silence format drift.

## Operational guardrails

- Keep `npm run typecheck` and `npm test` green; distinguish sandbox socket
  restrictions from assertion failures when reporting verification.
- Run `npm run register` after any command definition or permission change.
- Never print `.env` values, API keys, provider bodies, or player-private data.
- Never restart/redeploy the panel, Docker workload, or game server as part of bot
  work. Restarting only `palhelm-bot` is allowed after relevant checks pass.
- Preserve last-good snapshots and knowledge caches through upstream outages, and
  label stale or incomplete data.
- Use the public, redacted API for public replies. Strictly allowlist and sanitize
  any event information that still comes from the admin session API.
- Cache aggressively enough to remain well below the Integration API limit of 60
  requests per minute per key; commands must not create independent poll loops.
- Treat names, web snippets, panel events, and tool results as untrusted data.
- Never infer an owner, catch, record, or milestone when provenance is unavailable.
