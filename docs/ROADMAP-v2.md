# Roadmap — post-v0.3.0

Larger 1.0-era items that deliberately did NOT ship with the redesign, ranked by a
value/effort analysis.

## Blocked on upstream

- **`game-data` live world view** — `GET /v1/api/game-data` (World Actor Snapshot) still
  returned 404 on build `v1.0.0.100427` in the v0.3.0 GET-only re-probe. When it lands: live map overlays (pals,
  bases, PalBoxes), join/leave/death feed, guild grouping, actor-density vs FPS diagnostics.
  Re-probe after each server image update. Privacy: response includes player IPs/user-ids —
  redact by role, poll 10–30s.
- **1.0 map tiles — imagery fetched; coordinate correction pending deployment.** THGL (cdn.th.gl) is the only source
  with real 1.0 imagery (palworld.gg still serves stale pre-1.0 tiles as of 2026-07-10).
  `fetch-map-tiles.sh` now supports `--format webp`, `--transpose-yx` (THGL serves
  `{z}/{y}/{x}`; we store `{z}/{x}/{y}` so the served layout stays one convention),
  `--min-zoom`/`--max-zoom`/`--tile-size`, and multi-layer `dataset.json` (each `--layer` run
  merges its own entry via `.layers/*.json` fragments instead of clobbering prior layers).
  Both THGL layers are fetched and verified into
  `/your/palhelm-data/map-tiles-1.0/{default,tree}/` — 341 tiles each (z0–4,
  sum 4^z), zero zero-byte files. `backend/internal/server/tiles.go` serves both `.png` and
  `.webp` (content-type by extension) and both the legacy flat layout
  (`map-tiles/{z}/{x}/{y}.ext`) and the new layered layout
  (`map-tiles/{layer}/{z}/{x}/{y}.ext`); `GET /api/v1/map/dataset`'s `layers` array now
  carries each layer's format/tile_size/zoom range/transform/bounds.
  `frontend/src/routes/map/Map.tsx` drives tile URL/format/zoom/transform from that dataset,
  with a Palpagos/World Tree layer-picker (chip-toggle style, shown only when >1 layer is
  reported) and a "Map data: pre-1.0" stamp when `game_version != "1.0"`.

  A live-position audit on 2026-07-11 found the original integration interpreted THGL's
  coordinate order incorrectly and replaced its published offsets with offsets derived from
  bounds. Palworld data coordinates are axis-flipped: map/display X is derived from data Y,
  and map/display Y from data X. Local code now preserves THGL's full
  `L.Transformation(a,b,c,d)` and applies it to `(dataY,dataX)`. The exact display conversion
  is `x=(dataY-158000)/459`, `y=(dataX+123888)/459`. A real starting-area position now maps
  to approximately `(246,-500)` instead of the top-left corner, with forward/inverse
  regression coverage. The running panel and its dataset metadata remain unchanged until a
  deliberate rollout; regenerate/update `dataset.json` with the upstream offsets during that
  rollout.

- **Verified points of interest remain pending.** The old hardcoded fast-travel and boss-tower
  points were synthetic pre-1.0 guesses and have been removed from the local production map.
  Restore those layers only from a licensed, versioned 1.0 dataset with source attribution and
  coordinate validation. Refresh mock player/base positions separately; mock values must never
  be treated as surveyed game data.

## High demand, needs real design

1. **Verified restart service** — v0.3.0 truthfully ships only cancellable graceful shutdown.
   A future restart service needs a persisted timezone/missed-run schedule, automatic verified
   active-world backup, collision prevention, a proven host start capability, and observable
   post-shutdown success. Container-side Compose remains unsafe without a host helper.
2. **Discord webhooks** — join/leave/chat first (player-list diffing works today), deaths
   once game-data lands. Highest community demand across every bot/mod surveyed.
3. **Prometheus `/metrics` exporter** — passthrough of the REST metrics payload.
4. **Authoritative allow-list enforcement** — v0.3.0 relabels the existing SQLite list as local
   player notes. Defer enforcement to a supported vanilla mechanism or verified PalDefender
   adapter; do not infer access control from local annotations.
5. **Teleport via REST** — `TeleportToPlayer` is RCON-only; a small backend route unlocks it
   as a ⌘K palette action (palette ships in v2 without it by design).
6. **Guild-aware player/pal grouping** — cheap once game-data flows.

## Frontend follow-ups

- Toast a11y upgrade (pause-on-hover, assertive region for danger) — ~20 lines, fold into
  the next frontend touch.
- Player Combobox (searchable picker) on Base UI when a real call-site appears.
- Paldeck table regeneration tooling once a canonical 1.0 CharacterID→name dataset exists
  (community DBs disagreed on 286 vs 287 at launch; Astralym boss-class excluded pending
  asset-level verification).
- **Player gender/avatar — investigated, not built.** The frontend should ship a neutral
  default avatar per player; do not build a gender toggle on top of what's below.
  - What we checked: `backend/internal/sav/character.go`'s `characterFromEntry` reads a
    player's `CharacterSaveParameterMap` entry (`sp`) generically — `readProperties` walks
    every UE property until `None`, so any `Gender`-shaped field present in the save *would*
    already be sitting in that map; we just don't currently pull a name out of it into
    `sav.Player`. `backend/internal/sav/world.go`'s `loadPlayerDirectory` does the same generic
    read for the per-player `Players/<uid>.sav` `SaveData` struct.
  - We could not test this empirically against real data: the only fixture in
    `backend/internal/sav/testdata` (`Level.sav`) has zero player records
    (`TestParseLevel` asserts `len(w.Players) == 0`), and there is no `Players/*.sav` fixture
    in the repo at all.
  - Evidence from outside the repo, checked in lieu of a fixture: Palworld's own character
    creator does not label its customization choice "Gender" — it's "Type 1" / "Type 2" body
    presets (Pocketpair's own wording; the ambiguity is a recurring community complaint, see
    the Steam discussion "Changing 'Body Type 1/2' to 'Male or Female'"). Two actively
    maintained third-party save editors that do deep, field-by-field player parsing —
    `cheahjs/palworld-save-tools` (the reference GVAS/property reader nearly every Palworld
    tool, including ours, is compatible with) and `oMaN-Rod/palworld-save-pal` (an
    817-line `game/player.py` covering NickName, Level, Exp, HP, stomach, sanity, status
    points, quests, inventory, pal box, party, DPS stats) — expose **no** gender/body-type/
    appearance field for players. Both *do* expose `Gender` (`EPalGenderType`:
    Male/Female/Unknown), but only on Pals, for breeding compatibility — an unrelated field on
    an unrelated entity.
  - What it would take to build this for real: get hold of an actual `Players/<uid>.sav` from
    a live 1.0 server, run it through `cmd/savdump`, and manually diff the property names
    against what NickName/Level/Location already resolve, looking specifically for something
    body/mesh/voice-shaped under `PlayerCharacterMakeData` or similar — Pocketpair's own
    "Type 1/2" framing means whatever's there, if anything, is a body-type id, not a `male`/
    `female` string, and would need an explicit (and honestly somewhat arbitrary) mapping to
    present as "gender" in the API. Do not build that mapping speculatively; get a real save
    first.
