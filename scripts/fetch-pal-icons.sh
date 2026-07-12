#!/usr/bin/env sh
# Download per-Pal preview icons for the Paldeck / player-Pal-list screens.
#
# Pal icons are Pocketpair-derived art and are NOT distributed with Palhelm.
# Run this yourself; the icons land in your Palhelm data volume and stay on
# your machine. The default source is paldeck.cc's rendered Paldeck and direct
# icon paths. It currently exposes the complete 1.0-era roster and avoids one
# HTML page request per Pal. The older paldb.cc workflow remains as a fallback.
#
# For paldeck.cc, the roster is discovered from its rendered `/pals` page and
# therefore follows newly published icons without a Palhelm release. For the
# paldb.cc fallback, the roster still comes from `go run ./cmd/paldeck-list`.
#
# For each Pal this makes two requests: one for its paldb.cc wiki page (to
# read off the exact-case internal CharacterID paldb.cc embeds as its icon
# filename — image URLs are case-sensitive and paldeck.go only stores
# lowercased ids, so it can't be reconstructed without asking paldb.cc), and
# one to download the icon itself. Some Pals 404 or have no icon on their
# paldb.cc page (missing data on their end, or a display name paldb.cc slugs
# differently); those are logged to stderr and skipped, not treated as a
# fatal error.
#
# Usage: fetch-pal-icons.sh [dest-dir] [options]
#
#   [dest-dir]     positional dest dir, kept for back-compat; same as --dest
#                  (default: ./data/pal-icons)
#   --dest <dir>   destination dir for the icon set
#   --force        re-fetch every icon even if a non-empty file already
#                  exists at the destination. Without this, re-running the
#                  script only fills in gaps (previous 404s, new Pals added
#                  to paldeck.go).
#   --source <name> paldeck.cc (default) or paldb.cc
#   -h, --help     show this help and exit
#
# On completion, writes/updates a dataset.json sidecar in <dest-dir>
# recording source/fetched_at/count, per the format documented in
# backend/internal/server/paldeck_icons.go.
set -eu

DEST="./data/pal-icons"
SOURCE="paldeck.cc"
UA="palhelm-icon-fetch/1.0"
FORCE=0
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
BACKEND_DIR="$SCRIPT_DIR/../backend"

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
    --force) FORCE=1; shift ;;
    --source) SOURCE="$2"; shift 2 ;;
    -h|--help) usage 0 ;;
    *) echo "unknown argument: $1" >&2; usage 1 ;;
  esac
done

mkdir -p "$DEST"

case "$SOURCE" in
  paldeck.cc)
    roster_page="$(curl -fsSL --max-time 30 -A "$UA" https://api.paldeck.cc/pals)"
    roster="$(printf '%s' "$roster_page" \
      | grep -oE '/assets/palworld/pals/T_[A-Za-z0-9_]+_icon_normal\.webp' \
      | sed -E 's#^.*/T_##; s#_icon_normal\.webp$##' \
      | sort -fu)"
    ;;
  paldb.cc)
    roster="$(cd "$BACKEND_DIR" && go run ./cmd/paldeck-list)"
    ;;
  *)
    echo "unsupported source: $SOURCE (expected paldeck.cc or paldb.cc)" >&2
    exit 1
    ;;
esac
total=$(printf '%s\n' "$roster" | grep -c . || true)
[ "$total" -ge 200 ] || { echo "refusing implausibly small icon roster ($total)" >&2; exit 1; }
echo "Fetching up to $total Pal icons into $DEST from $SOURCE ..."
if [ "$FORCE" -eq 1 ]; then
  echo "  --force: re-downloading every icon, even ones already present"
fi

fetched=0
skipped=0
missing=0
missing_ids=""
rm -f "$DEST/.fetched.tmp" "$DEST/.missing.tmp"

# paldeck.cc emits one ID per line; paldb.cc emits "id<TAB>name".
printf '%s\n' "$roster" | while IFS="$(printf '\t')" read -r id name; do
  [ -n "$id" ] || continue
  file_id="$(printf '%s' "$id" | tr '[:upper:]' '[:lower:]')"
  out="$DEST/$file_id.webp"
  if [ "$FORCE" -ne 1 ] && [ -s "$out" ]; then
    echo "skip $id (already fetched)"
    continue
  fi

  if [ "$SOURCE" = "paldeck.cc" ]; then
    icon_url="https://api.paldeck.cc/assets/palworld/pals/T_${id}_icon_normal.webp"
  else
    slug="$(printf '%s' "$name" | tr ' ' '_')"
    page="$(curl -fsSL --max-time 15 -A "$UA" "https://paldb.cc/en/$slug" 2>/dev/null || true)"
    sleep 0.15
    icon_path="$(printf '%s' "$page" | grep -oE 'PalIcon/Normal/T_[A-Za-z0-9_]+_icon_normal\.webp' | head -1)"
    if [ -z "$icon_path" ]; then
      echo "warn: no icon found for $id ($name), slug=$slug" >&2
      echo "$file_id" >> "$DEST/.missing.tmp"
      continue
    fi
    icon_url="https://cdn.paldb.cc/image/Pal/Texture/$icon_path"
  fi

  if curl -fsSL --max-time 15 -A "$UA" "$icon_url" -o "$out"; then
    echo "fetched $id${name:+ ($name)}"
    echo "$file_id" >> "$DEST/.fetched.tmp"
  else
    echo "warn: download failed for $id ($name)" >&2
    rm -f "$out"
    echo "$file_id" >> "$DEST/.missing.tmp"
  fi
  sleep 0.15
done

fetched=0
[ -f "$DEST/.fetched.tmp" ] && fetched=$(wc -l < "$DEST/.fetched.tmp" | tr -d ' ')
missing=0
[ -f "$DEST/.missing.tmp" ] && missing=$(wc -l < "$DEST/.missing.tmp" | tr -d ' ')
present=$(find "$DEST" -maxdepth 1 -name '*.webp' -size +0c | wc -l | tr -d ' ')
rm -f "$DEST/.fetched.tmp" "$DEST/.missing.tmp"

fetched_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cat > "$DEST/dataset.json" <<EOF
{"source":"$SOURCE","fetched_at":"$fetched_at","count":$present}
EOF

echo "Done: $fetched newly fetched this run, $missing missing/failed this run, $present total icons on disk."
echo "Wrote dataset.json into $DEST."
echo "Point PALHELM_PAL_ICONS_DIR at $DEST (or keep the default data dir layout)."
