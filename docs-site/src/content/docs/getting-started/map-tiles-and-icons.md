---
title: Map tiles and Pal icons
description: Why map tiles and Pal icons are not bundled, how the two fetch scripts work, and where the files land in your data volume.
sidebar:
  order: 4
---

This page covers the two fetch scripts that populate the live map and the Pal art. It explains why these assets are not shipped with Palhelm, what each script downloads, and where the files land in your data volume.

## Why these are not bundled

Map imagery and Pal icons are derived from Pocketpair's game assets. Palhelm does not redistribute them. Instead, you run a one-time script that downloads the assets to your own data volume, where they stay on your machine. Until you run the scripts, the live map and the Pal art are empty. Everything else in the panel works without them.

Both scripts write a `dataset.json` sidecar next to the downloaded files. Palhelm reads that sidecar to learn what is present. You find both scripts in the repository under `scripts/`.

## Map tiles

`scripts/fetch-map-tiles.sh` downloads a pyramid of map tiles for the live map. The simplest run downloads the default source into your data volume:

```sh
scripts/fetch-map-tiles.sh ./palhelm-data/map-tiles
```

The default source is a flat `z/x/y` PNG pyramid covering zoom levels 0 through 6. On completion the script writes a `dataset.json` into the destination recording the source, the fetch time, and the game version.

### Palworld 1.0 tiles

For 1.0-era imagery, the alternate source (THGL) splits the world into named layers, each its own WebP pyramid. You run the script once per layer, and each run merges its own entry into a shared `dataset.json`. The two layers are Palpagos (the `default` layer) and the World Tree (the `tree` layer). The repository's script header carries the exact commands, including the per-layer base URLs and the transform values.

Because these are separate WebP pyramids served in a different path order, the 1.0 runs pass extra flags. In short:

- `--base` is the complete tile root for that layer. Each layer has its own hashed URL, not a shared base with a subpath.
- `--format webp` sets the image format.
- `--transpose-yx` tells the script the remote serves tiles as `z/y/x`. The script still stores them locally as `z/x/y`, so Palhelm always reads one layout.
- `--layer` and `--label` name the layer for `dataset.json`.
- `--tile-size`, `--min-zoom`, and `--max-zoom` describe the pyramid.
- `--transform` and `--bounds` carry the layer's coordinate mapping. Preserve all four transform values exactly. They are not derivable from the bounds.

Re-running the script is a no-op for tiles that are already present, so it is safe to run again to fill gaps. Pass `--force` to re-download every tile even when a file already exists.

### Where map tiles land

The tiles and the `dataset.json` land under the destination you pass, which should be inside the `/data` volume. In the install example the host path is `../palhelm-data/map-tiles`, which is `/data/map-tiles` inside the container. Palhelm serves tiles from the `map-tiles` directory inside its data folder, so keep that layout. (The fetch script's closing message mentions a `PALHELM_MAP_TILES` variable, but the panel does not read it.)

## Pal icons

`scripts/fetch-pal-icons.sh` downloads a per-Pal preview icon for the Paldeck and the per-player Pal lists. Run it against a directory inside your data volume:

```sh
scripts/fetch-pal-icons.sh ./palhelm-data/pal-icons
```

The default source exposes the complete 1.0-era roster. The script refuses to continue if it discovers an implausibly small roster, as a guard against a broken source. It downloads one WebP icon per Pal, lowercases each id for the filename, and writes a `dataset.json` recording the source, the fetch time, and the count on disk.

Some Pals may have no icon at the source. Those are logged and skipped, not treated as a fatal error. Re-running the script only fills in gaps, such as previous misses or newly published Pals. Pass `--force` to re-download every icon.

An older fallback source (`--source paldb.cc`) exists if the default source is unavailable. It reads the roster with a helper command and makes an extra request per Pal to read the exact-case icon filename.

### Where Pal icons land

The icons and the `dataset.json` land in the destination directory, which should be inside the `/data` volume. In the install example that is `../palhelm-data/pal-icons` on the host, or `/data/pal-icons` inside the container. Palhelm serves icons from the `pal-icons` directory inside its data folder, so keep that layout. (The fetch script's closing message mentions a `PALHELM_PAL_ICONS_DIR` variable, but the panel does not read it.)

## Run them from the host

Both scripts use `curl` and standard shell tools and are meant to run on the host, writing into the same data directory the container mounts at `/data`. Run them once after install, then again only when you want to refresh or fill gaps. Palhelm picks up the new files without a restart on the next relevant screen load.
