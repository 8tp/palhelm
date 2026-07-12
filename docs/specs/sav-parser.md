# Spec: `internal/sav` — Palworld 1.0 save parser (Go, decode-only)

## Goal
A pure-Go package that parses Palworld `Level.sav` / `LevelMeta.sav` (1.0 format) into typed
structs: players, pals, guilds, base camps. Read-only — no re-encoding.

## Module layout
- Go module `github.com/palhelm/palhelm`, rooted at `backend/`.
- Package path: `backend/internal/sav`.
- CLI for manual testing: `backend/cmd/savdump/main.go` — `savdump <file.sav>` prints parsed JSON.

## Container format (validated live)
Header: `[u32le uncompressed_len][u32le compressed_len][3-byte magic][1 byte save_type]`
- Magic `PlZ`: save_type `0x31` = zlib once, `0x32` = zlib twice (decompress outer then inner). Pre-0.6 saves.
- Magic `PlM`: save_type `0x31` = Oodle (Mermaid) — current format. Body starts at offset 12.
- Some console saves prepend a 12-byte `CNK` chunk; if magic at offset 8..11 isn't PlZ/PlM, retry at offset 20..23 (skip 12 bytes).
- Decompressed body must begin with `GVAS`.

## Oodle
Use `github.com/new-world-tools/go-oodle` (MIT, purego-based, no CGO). Behavior:
1. If env `PALHELM_OODLE_LIB` is set, load that shared library path.
2. Else look for `liboo2corelinux64.so.9` in `PALHELM_DATA_DIR` (default `./data`).
3. Else call the library's Download helper to fetch it there (log this clearly). Never bundle it.
Wrap in `sav/oodle.go` with a single `oodleDecompress(src []byte, rawLen int) ([]byte, error)`.

## GVAS reader
Decode-only port of the property tree reader from the oMaN-Rod fork of palworld-save-tools
(Python reference installed at
`<scratch>/savtest/lib/python3.12/site-packages/palworld_save_tools/`
— read `archive.py`, `gvas.py`, `paltypes.py`, `rawdata/character.py`, `rawdata/group.py`).

Requirements:
- Header struct (save_game_version, package version, engine version, custom format data), then
  properties-until-end loop. Support property types: Int, Int64, UInt32, Float, Double, Bool, Byte,
  Enum, Str, Name, Struct (incl. Vector, Quat, LinearColor, DateTime, Guid), Array, Map, and raw
  fallback for anything unknown (skip by size, count skips).
- **Type hints**: port `PALWORLD_TYPE_HINTS` for the paths we traverse (empty-map/array typing).
- **Custom RawData decoders — ONLY these two** (everything else stays `[]byte`):
  - `.worldSaveData.CharacterSaveParameterMap.Value.RawData` → inner GVAS properties
    (`SaveParameter` struct: discriminate players via `IsPlayer`; extract NickName, Level, Exp,
    HP, character_id, OwnerPlayerUId for pals, etc.)
  - `.worldSaveData.GroupSaveDataMap` → guild raw blob (group_type, group_id, group_name,
    individual_character_handle_ids, org type; for guilds: base_ids, admin_player_uid, players
    with last_online/name). **Port from the oMaN-Rod fork's `group.py`, not cheahjs 0.24.0 —
    the 1.0 layout differs and the old decoder fails.** Our live save is the conformance test.
- Tolerant: unknown property/struct → skip with counter (exposed as `ParseStats`), never panic;
  guard all reads against EOF (return error, not slice-panic).

## Public API
```go
type World struct {
    Meta     WorldMeta   // from LevelMeta.sav: world name, day, etc.
    Players  []Player    // uid, nickname, level, lastOnline, guildId, location (if present)
    Pals     []Pal       // identity/owner/placement plus HP, gender, talents, passives, equipped attacks
    Guilds   []Guild     // id, name, adminUid, memberUids, baseIds, baseCampLevel
    Bases    []BaseCamp  // id, guildId, position
    Stats    ParseStats  // skipped properties, decode failures per section
}
func ParseLevel(path string, opts Options) (*World, error)      // Level.sav (+ optional Players/ dir)
func ParseLevelMeta(path string) (*WorldMeta, error)
```
`Players/` dir may not exist (no player has joined yet) — that's a normal state: empty slice.
`CharacterSaveParameterMap` may be absent — same.

## Palworld 1.0 drift status (2026-07-11 read-only probe)

The current live save decodes players, Pals, guilds, placement, and the populated
`DungeonSaveData[].RewardSaveDataMap`. That reward map declares struct keys but
stores a bare GUID; the parser has a path-specific `Guid` hint and a synthetic
non-empty regression test.

One drift counter remains: one guild record has a 145-byte opaque tail that does
not match the proven 14/31, 4/4, or 0/0 layouts. Exhaustive diagnostic trials
found only ambiguous zero-member alignments, so the parser deliberately keeps
the guild through its tolerant fallback and reports
`GroupSaveDataMap.Value.RawData.guild-tail (tolerated)`. Do not suppress that
counter or promote an arbitrary alignment until a deterministic field layout is
identified and captured in a synthetic fixture.

## Tests (must pass; live fixtures)
Fixtures dir: `<scratch>/`
- `Level.sav` — live 1.0 save. Expect: decompresses to exactly 278289 bytes starting `GVAS`;
  parse yields **7 guild-map entries** and 0 players; no panics.
- `Level.gvas` — the pre-decompressed body (ground truth for the Oodle step: byte-compare).
- `LevelMeta.sav` — parses; contains world meta (world name "Autosave_W" for the committed
  fixture — verify the actual field and assert on it).
- Unit tests for the container header logic (PlZ/PlM/CNK) with tiny synthetic zlib fixtures.
- Copy the fixtures into `backend/internal/sav/testdata/` (they're small) so tests are hermetic.
- A benchmark on Level.sav parse (guards the no-RAM-blowup goal).

## Conventions
- The sav package stays dependency-light: standard library plus go-oodle and purego only.
- Code is split into small files by concern (container.go, oodle.go, gvas.go, props.go,
  character.go, group.go, world.go), gofmt and vet clean, with the exported API documented.
