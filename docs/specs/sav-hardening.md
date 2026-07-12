# Spec: sav parser + Oodle loader hardening

Adversarial review findings (Grok 4.5, 2026-07-10) on `internal/sav` and
`third_party/go-oodle`. Threat model: corrupted or hostile .sav input (DoS via allocation),
and local-privilege attacks on the native-library load path (code execution). Fix all HIGH
and MEDIUM items; LOW at your judgment. Keep all existing tests green; add tests where noted.

## High

1. `container.go` `zlibBytes`: `io.ReadAll` uncapped → zip bomb. Use
   `io.LimitReader(zr, int64(h.RawLen)+1)` and error if output exceeds `RawLen`
   (apply to both passes of the 0x32 double-zlib path; cap the intermediate too).
2. `container.go`/`oodle.go`: `RawLen` is attacker-controlled u32 driving `make([]byte, n)`.
   Enforce a global max decompressed size: `PALHELM_SAV_MAX_BYTES` env, default 2 GiB hard
   cap and 512 MiB default, checked BEFORE allocation. Same cap on the whole-file
   `os.ReadFile` (Stat first, reject oversized).
3. `props.go` array/map/custom-version counts: never preallocate from a file-supplied count.
   Bound `n` by `remaining / minElementSize` and grow slices with append (start cap 0 or
   small). Applies at ~L255, L300, L331, `gvas.go` L93, `group.go` count sites.
4. **Oodle load path — rework `third_party/go-oodle` + `sav/oodle.go` together:**
   - Add `oodle.LoadFrom(absPath string) error` that Dlopens exactly the given path.
     Delete the `/tmp/go-oodle` staging/symlink dance in `sav/oodle.go` (`exposeOodleLibrary`
     goes away).
   - Load order: `PALHELM_OODLE_LIB` (absolute path required) → `<PALHELM_DATA_DIR>/liboo2corelinux64.so.9`.
   - Download (only when neither exists): fetch to `<dataDir>/.oodle.tmp-<rand>` with
     `http.Client{Timeout: 60s}`, verify **SHA-256 pinned constant** (compute it once from the
     actual release artifact at implementation time and hard-code it with a comment),
     `chmod 0755`, atomic rename into place. On any failure remove the temp file and return
     an error (never a hang: the sync.Once must complete with error).
   - `RTLD_LOCAL` instead of `RTLD_GLOBAL` if purego permits.
5. Recursion/size limits: cap property-tree depth (64) and total property count (4M) in the
   until-None loops; return a typed error, not a panic/OOM.

## Medium

6. `props.go` unknown-property skip: current skip desyncs (known types consume an
   optional-guid byte the skip path doesn't). Read the optional-guid flag byte before
   skipping `size` bytes, and if the skip lands past EOF, fail the parse (don't limp).
7. `props.go` struct fallback re-seek: for nested (map/array) struct errors, propagate the
   error instead of size-skipping with a wrong/zero size.
8. `reader.go` FString cap: lower to 4 MiB per string; compare lengths in int64.
9. UTF-16 FString: validate trailing NUL like the ANSI path (consistency).

## Tests to add
- Fuzz-style table tests: truncated file at every 16-byte boundary of the fixture header;
  count fields patched to 0xFFFFFFFF (array, map, string, custom-version) → parser must
  return an error quickly, never panic and never allocate > cap (assert with
  `testing.AllocsPerRun` or a runtime.MemStats delta guard on the worst case).
- Zlib-bomb fixture: small PlZ save whose stream inflates past RawLen → error.
- Oodle loader: table test for the resolve order using temp dirs (no network in tests;
  fake the download with an httptest server + a test-only hash override).

Run `go test ./... -count=1` and `go vet ./...`. Do not change the public sav API.
Update docs/ARCHITECTURE.md's Oodle paragraph if the load-order behavior changes.
