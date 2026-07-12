#!/usr/bin/env sh
# Download the Palworld world-map tile pyramid for the Live Map screen.
#
# Map imagery is derived from Pocketpair's game assets and is NOT distributed
# with Palhelm. Run this yourself; the tiles land in your Palhelm data volume
# and stay on your machine. Default tile source: palworld.gg (z 0-6, XYZ scheme,
# PNG, flat layout). THGL (cdn.th.gl) is an alternate source with 1.0-era
# imagery split across named layers (e.g. "default"/Palpagos, "tree"/World
# Tree), each a separate z0-4 512px WebP pyramid served in z/y/x URL order —
# use --format, --transpose-yx, --layer, --tile-size, --max-zoom and
# --transform/--bounds together for those (see examples below).
#
# Usage: fetch-map-tiles.sh [dest-dir] [options]
#
#   [dest-dir]            positional dest dir, kept for back-compat; same as
#                         --dest (default: ./data/map-tiles)
#   --dest <dir>          destination dir for the tile pyramid
#   --base <url>          tile source base URL, without the {z}/{x}/{y} suffix
#                         (default: palworld.gg pyramid)
#   --format <png|webp>   remote/local tile image format (default: png)
#   --transpose-yx        the remote source serves tiles at {z}/{y}/{x}
#                         (row before column, e.g. THGL); download that path
#                         order but always store locally as {z}/{x}/{y} so the
#                         layout served by Palhelm stays one convention
#   --tile-size <px>      recorded in dataset.json for the frontend/backend;
#                         purely informational, does not affect fetching
#                         (default: 256)
#   --min-zoom <n>        first native zoom level to fetch (default: 0)
#   --max-zoom <n>        last native zoom level to fetch (default: 6)
#   --layer <name>        named layer (e.g. THGL's "default"/"tree"); appended
#                         as a path segment to --dest only (pass --base the
#                         complete tile root for that layer — sources like
#                         THGL give each layer its own hashed URL, not a
#                         shared-base subpath), and recorded as an entry in
#                         dataset.json's "layers" array at <dest-dir>/dataset.json
#                         (NOT nested inside the layer dir). Re-run once per
#                         layer to build up a multi-layer dataset.json — each
#                         run merges its own layer entry in without disturbing
#                         prior ones.
#   --label <text>        human-readable label for --layer's dataset.json
#                         entry (e.g. "World Tree"); defaults to the --layer
#                         value
#   --transform <a,b,c,d> optional world-coord -> tile-pixel transform for
#                         this layer's dataset.json entry, Leaflet
#                         L.Transformation convention: pixel(z) = 2^z *
#                         (a*dataY + b), 2^z * (c*dataX + d), canvas at
#                         native zoom 0 is tile-size square.
#                         THGL publishes this array directly. Preserve all four
#                         values verbatim: its offsets are not derivable from the
#                         fit bounds. THGL/Leaflet horizontal input is Palworld
#                         data Y and vertical input is data X (axes are flipped).
#   --bounds <x1,y1,x2,y2> optional world-coord bounding box for this layer's
#                         dataset.json entry, informational only
#   --game-version <ver>  game_version string recorded in dataset.json
#                         (default: unknown)
#   --source <name>       source string recorded in dataset.json's top-level
#                         "source" field (default: derived from --base)
#   --notes <text>        free-form caveat recorded in dataset.json's
#                         top-level "notes" field (e.g. a known offset issue)
#   --force               re-fetch every tile even if a non-empty file
#                         already exists at the destination. Without this,
#                         re-running the script is a no-op against already-
#                         populated tiles even if upstream has re-rendered.
#   -h, --help            show this help and exit
#
# Examples:
#   # legacy palworld.gg pyramid (unchanged default behavior)
#   ./fetch-map-tiles.sh /data/map-tiles
#
#   # THGL 1.0 Palpagos + World Tree layers, transform from cdn.th.gl's own
#   # config/tiles.json (see docs/ROADMAP-v2.md for how these were derived)
#   ./fetch-map-tiles.sh --dest /data/map-tiles-1.0 \
#     --base https://cdn.th.gl/palworld/map-tiles/default-733001e0986faa3f88b0a970412d7fb9 \
#     --format webp --transpose-yx --tile-size 512 --min-zoom 0 --max-zoom 4 \
#     --layer default --label Palpagos --source thgl --game-version 1.0 \
#     --transform 0.000353395913859746,256,-0.000353395913859746,123.47653230259525 \
#     --bounds -1099399,-724399,349399,724399
#   ./fetch-map-tiles.sh --dest /data/map-tiles-1.0 \
#     --base https://cdn.th.gl/palworld/map-tiles/tree-bd046c3cfb06ee41b25a111f912d407f \
#     --format webp --transpose-yx --tile-size 512 --min-zoom 0 --max-zoom 4 \
#     --layer tree --label "World Tree" --source thgl --game-version 1.0 \
#     --transform 0.0014979651664584533,1225.6306053008072,-0.0014979651664584533,1032.3204475170935 \
#     --bounds 347352.5,-818196,689147.5,-476401
#
# On completion, writes/updates a dataset.json sidecar in <dest-dir>
# recording source/fetched_at/game_version/notes (+ layers), per the format
# documented in backend/internal/server/tiles.go.
set -eu

DEST="./data/map-tiles"
BASE="https://palworld.gg/images/tiles"
FORMAT="png"
TRANSPOSE_YX=0
TILE_SIZE=256
MIN_ZOOM=0
MAX_ZOOM=6
LAYER=""
LABEL=""
TRANSFORM=""
BOUNDS=""
FORCE=0
GAME_VERSION="unknown"
SOURCE=""
NOTES=""
UA="palhelm-tile-fetch/1.0"

usage() {
  awk 'NR>1 && /^#/{sub(/^# ?/,""); print; next} NR>1 && !/^#/{exit}' "$0"
  exit "${1:-0}"
}

# positional dest-dir, kept for back-compat, if the first arg isn't a flag
if [ "$#" -gt 0 ]; then
  case "$1" in
    -*) ;;
    *) DEST="$1"; shift ;;
  esac
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dest) DEST="$2"; shift 2 ;;
    --base) BASE="$2"; shift 2 ;;
    --format) FORMAT="$2"; shift 2 ;;
    --transpose-yx) TRANSPOSE_YX=1; shift ;;
    --tile-size) TILE_SIZE="$2"; shift 2 ;;
    --min-zoom) MIN_ZOOM="$2"; shift 2 ;;
    --max-zoom) MAX_ZOOM="$2"; shift 2 ;;
    --layer) LAYER="$2"; shift 2 ;;
    --label) LABEL="$2"; shift 2 ;;
    --transform) TRANSFORM="$2"; shift 2 ;;
    --bounds) BOUNDS="$2"; shift 2 ;;
    --game-version) GAME_VERSION="$2"; shift 2 ;;
    --source) SOURCE="$2"; shift 2 ;;
    --notes) NOTES="$2"; shift 2 ;;
    --force) FORCE=1; shift ;;
    -h|--help) usage 0 ;;
    *) echo "unknown argument: $1" >&2; usage 1 ;;
  esac
done

case "$FORMAT" in
  png|webp) ;;
  *) echo "--format must be png or webp, got: $FORMAT" >&2; exit 1 ;;
esac

ROOT_DEST="$DEST"
if [ -n "$LAYER" ]; then
  case "$LAYER" in
    *[!a-zA-Z0-9_-]*) echo "--layer must be alphanumeric/underscore/hyphen only, got: $LAYER" >&2; exit 1 ;;
  esac
  # --layer only affects the local destination + dataset.json bookkeeping, not
  # --base: pass --base the complete tile root for that layer (THGL, for
  # instance, gives each layer its own hashed URL, not a shared-base subpath).
  DEST="$DEST/$LAYER"
fi
if [ -z "$SOURCE" ]; then
  SOURCE="$BASE"
fi
if [ -z "$LABEL" ]; then
  LABEL="$LAYER"
fi

total=0
z="$MIN_ZOOM"
while [ "$z" -le "$MAX_ZOOM" ]; do
  n=$((1 << z))
  total=$((total + n * n))
  z=$((z + 1))
done

echo "Fetching $total tiles (z$MIN_ZOOM-$MAX_ZOOM, .$FORMAT) into $DEST ..."
if [ "$TRANSPOSE_YX" -eq 1 ]; then
  echo "  --transpose-yx: remote path is {z}/{y}/{x}, stored locally as {z}/{x}/{y}"
fi
if [ "$FORCE" -eq 1 ]; then
  echo "  --force: re-downloading every tile, even ones already present"
fi
processed=0
downloaded=0
skipped=0
failed=0
z="$MIN_ZOOM"
while [ "$z" -le "$MAX_ZOOM" ]; do
  n=$((1 << z))
  y=0
  while [ "$y" -lt "$n" ]; do
    x=0
    while [ "$x" -lt "$n" ]; do
      out="$DEST/$z/$x/$y.$FORMAT"
      mkdir -p "$DEST/$z/$x"
      if [ "$FORCE" -eq 1 ] || [ ! -s "$out" ]; then
        if [ "$TRANSPOSE_YX" -eq 1 ]; then
          src="$BASE/$z/$y/$x.$FORMAT"
        else
          src="$BASE/$z/$x/$y.$FORMAT"
        fi
        if curl -fsSL -A "$UA" "$src" -o "$out"; then
          downloaded=$((downloaded + 1))
        else
          echo "warn: tile $z/$x/$y failed ($src)" >&2
          rm -f "$out"
          failed=$((failed + 1))
        fi
        # be polite to the tile host
        sleep 0.05
      else
        skipped=$((skipped + 1))
      fi
      processed=$((processed + 1))
      x=$((x + 1))
    done
    y=$((y + 1))
  done
  echo "  zoom $z done ($processed/$total)"
  z=$((z + 1))
done

fetched_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# ---- dataset.json ----
# Legacy behavior (no --layer): a flat single-pyramid dataset.json with an
# empty layers array, written straight into $DEST. Unchanged for back-compat.
#
# Layered behavior (--layer given): dataset.json lives at $ROOT_DEST (the
# parent of the per-layer subdirs, matching what tiles.go actually reads),
# and each layer's fetch appends/replaces its own entry in the "layers"
# array without disturbing entries other layers previously wrote. This is
# done via small per-layer JSON fragment files cached in $ROOT_DEST/.layers/
# (each one already a complete, valid JSON object) that get concatenated
# into the top-level file's "layers" array — no jq/python dependency needed.
if [ -z "$LAYER" ]; then
  mkdir -p "$DEST"
  cat > "$DEST/dataset.json" <<EOF
{"source":"$SOURCE","fetched_at":"$fetched_at","game_version":"$GAME_VERSION","layers":[]}
EOF
else
  fragdir="$ROOT_DEST/.layers"
  mkdir -p "$fragdir"

  # notes is a top-level (not per-layer) field, but each layer's run only knows its own
  # --notes; cache it across runs the same way layer fragments are, so re-running for a
  # second layer without repeating --notes doesn't drop a caveat an earlier run recorded.
  notes_cache="$fragdir/.notes"
  if [ -n "$NOTES" ]; then
    printf '%s' "$NOTES" > "$notes_cache"
  elif [ -f "$notes_cache" ]; then
    NOTES="$(cat "$notes_cache")"
  fi

  transform_json="null"
  if [ -n "$TRANSFORM" ]; then
    a=$(echo "$TRANSFORM" | cut -d, -f1)
    b=$(echo "$TRANSFORM" | cut -d, -f2)
    c=$(echo "$TRANSFORM" | cut -d, -f3)
    d=$(echo "$TRANSFORM" | cut -d, -f4)
    transform_json="{\"a\":$a,\"b\":$b,\"c\":$c,\"d\":$d}"
  fi

  bounds_json="null"
  if [ -n "$BOUNDS" ]; then
    bx1=$(echo "$BOUNDS" | cut -d, -f1)
    by1=$(echo "$BOUNDS" | cut -d, -f2)
    bx2=$(echo "$BOUNDS" | cut -d, -f3)
    by2=$(echo "$BOUNDS" | cut -d, -f4)
    bounds_json="[[$bx1,$by1],[$bx2,$by2]]"
  fi

  cat > "$fragdir/$LAYER.json" <<EOF
{"id":"$LAYER","label":"$LABEL","path":"$LAYER","format":"$FORMAT","tile_size":$TILE_SIZE,"min_zoom":$MIN_ZOOM,"max_zoom":$MAX_ZOOM,"transform":$transform_json,"bounds":$bounds_json}
EOF

  layers_json="["
  first=1
  for f in "$fragdir"/*.json; do
    [ -e "$f" ] || continue
    if [ "$first" -eq 1 ]; then
      first=0
    else
      layers_json="$layers_json,"
    fi
    layers_json="$layers_json$(cat "$f")"
  done
  layers_json="$layers_json]"

  notes_field=""
  if [ -n "$NOTES" ]; then
    notes_field=",\"notes\":\"$NOTES\""
  fi

  mkdir -p "$ROOT_DEST"
  cat > "$ROOT_DEST/dataset.json" <<EOF
{"source":"$SOURCE","fetched_at":"$fetched_at","game_version":"$GAME_VERSION"$notes_field,"layers":$layers_json}
EOF
fi

echo "Done: $downloaded downloaded, $skipped skipped, $failed failed."
echo "Wrote dataset.json into $ROOT_DEST."
echo "Point PALHELM_MAP_TILES at $ROOT_DEST (or keep the default data dir layout)."
