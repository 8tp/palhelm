---
title: Save parser
description: How Palhelm reads Palworld 1.0 save files. Oodle, GVAS, what it parses, what it skips, and how it degrades.
sidebar:
  order: 3
---

This page covers the save parser at a technically honest level: the compressed
container, the GVAS property tree, which parts Palhelm decodes and which it leaves as
opaque bytes, how it degrades when the format drifts, and how the proprietary
decompressor is obtained without shipping it.

The parser is a pure-Go package. It decodes only. It never re-encodes or writes a save.

## The container

A Palworld save file starts with a small header, then a compressed body:

```text
[u32 uncompressed_len][u32 compressed_len][3-byte magic][1 byte save_type]
```

The magic and save type select the decompression path:

- Magic `PlM`, save type `0x31`: Oodle Mermaid compression. This is the current 1.0
  format. The body starts at offset 12.
- Magic `PlZ`, save type `0x31`: zlib, once. Save type `0x32`: zlib, twice. These are
  older, pre-1.0 saves. Palhelm keeps a zlib path for them.
- Some console saves prepend a 12-byte chunk. If the magic is not at offset 8, Palhelm
  retries 12 bytes later.

Once decompressed, the body must begin with `GVAS`. If it does not, the parser stops
rather than guessing.

## Oodle decompression

The 1.0 format uses Oodle Mermaid. The Oodle decompressor is proprietary and cannot be
redistributed, so Palhelm never bundles it in the repository or the Docker image.
Instead it obtains the shared library at run time, on first save parse, in this order:

1. If `PALHELM_OODLE_LIB` is set, load that path.
2. Otherwise look for `liboo2corelinux64.so.9` in the data directory.
3. Otherwise download it into the data directory, then verify it against a pinned
   SHA-256 before loading it.

The download is atomic and the checksum is checked before the library is used. On an
air-gapped host, an operator can drop the file into the data directory by hand or point
`PALHELM_OODLE_LIB` at it. The library is loaded through purego with `dlopen`, so the
Go binary itself stays CGO-free. On the musl-based Alpine runtime image, `gcompat` and
`libstdc++` let the process load this glibc library.

## The GVAS property tree

The decompressed body is standard Unreal Engine GVAS, engine version
`++UE5+Release-5.1`. Palhelm ports a decode-only reader of the property tree, using the
maintained oMaN-Rod fork of palworld-save-tools as the reference.

The reader handles the property types that appear on the paths Palhelm traverses: Int,
Int64, UInt32, Float, Double, Bool, Byte, Enum, Str, Name, Struct (including Vector,
Quat, LinearColor, DateTime, and Guid), Array, and Map. Anything unknown is skipped by
its declared size and counted, rather than causing a failure.

## What Palhelm parses, and what it skips

Palworld stores several sections as opaque `RawData` blobs inside the property tree.
Palhelm decodes only the three it needs and leaves everything else as bytes.

Parsed:

- `CharacterSaveParameterMap`: players and pals, told apart by an `IsPlayer` flag.
  Palhelm reads identity, level, HP, character id, owner, placement, gender, talents,
  passive skills, equipped attacks, and the Pal Condenser rank.
- `GroupSaveDataMap`: guilds, including id, name, admin, members with last-online
  timestamps, and base ids. The 1.0 layout of this blob differs from older saves, so
  Palhelm follows the maintained oMaN-Rod fork here. The older cheahjs decoder fails on
  the 1.0 layout.
- `BaseCampSaveData`: each base camp's player-chosen name and its world-space location,
  decoded from the base's own `RawData` (along with the worker-container GUID that links
  base-worker Pals). An unnamed base — the engine writes an untranslated placeholder into
  every base the player never renamed — decodes to `null`, never a synthetic label; a base
  whose transform fails to decode serves a `null` location, never a misleading `(0,0)`.

Left as opaque bytes:

- Item containers.
- Foliage.
- Dungeons, apart from a small reward map that the parser does read.

This is deliberate. Item, foliage, and dungeon blobs are large, and Palhelm does not
need them. Decoding them is what drives the multi-gigabyte memory spikes that the Python
tools hit on large saves. By skipping them, Palhelm keeps parse memory bounded. A
benchmark on a real save guards against regressions here.

## Degrading when the format drifts

The parser is tolerant by design. It is built to lose one feature rather than the whole
panel when a save does not match expectations:

- An unknown property or struct is skipped and counted in a `ParseStats` record. The
  count of skipped properties is capped so a pathological file cannot drive unbounded
  memory use through the skip accounting itself.
- Every read is guarded against reaching the end of the buffer. A short or corrupt blob
  returns an error instead of panicking.
- If a sub-decoder fails, only that section degrades. The panel keeps working and shows a
  save format drift indicator so operators know some data may be incomplete.

The current live 1.0 save decodes players, pals, guilds, placement, and a populated
dungeon reward map cleanly. One tolerated drift counter remains: a single guild record
has a short opaque tail that does not match the known layouts. The parser keeps that
guild through its tolerant fallback and reports the tail rather than dropping the record
or guessing at a layout. It will not promote a guessed alignment until a deterministic
field layout is identified and captured in a test fixture.

## Where the code lives

The package is `backend/internal/sav`, split into small files by concern: the container
reader, the Oodle wrapper, the GVAS reader, the property decoders, the character and
group RawData decoders, and the world assembler. There is a small command,
`backend/cmd/savdump`, that parses a file and prints the result as JSON for manual
inspection. The design and conformance notes live in `docs/specs/sav-parser.md` in the
repository.
