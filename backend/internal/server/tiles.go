package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// Matches both layouts tilesHandler serves:
//   - legacy flat:  /map-tiles/{z}/{x}/{y}.{png,webp}                (pre-1.0 palworld.gg pyramid)
//   - layered:      /map-tiles/{layer}/{z}/{x}/{y}.{png,webp}        (THGL 1.0+, one dir per layer)
//
// The optional layer segment is restricted to [a-z0-9_-] (matching the fetch script's own
// validation of --layer) so it can never carry a path-traversal or extra-segment payload.
var tilePathRe = regexp.MustCompile(`^/map-tiles/(?:([a-z0-9][a-z0-9_-]*)/)?(\d{1,3})/(\d{1,3})/(\d{1,3})\.(png|webp)$`)

var tileContentTypes = map[string]string{"png": "image/png", "webp": "image/webp"}

// tilesHandler serves the operator-downloaded map tile pyramid from
// <dataDir>/map-tiles. Tiles are game-derived art and are never bundled;
// a 404 here is how the frontend detects "tiles not installed".
func (s *Server) tilesHandler() http.Handler {
	root := filepath.Join(s.cfg.DataDir, "map-tiles")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := tilePathRe.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		layer, z, x, y, ext := m[1], m[2], m[3], m[4], m[5]
		p := filepath.Join(root, layer, z, x, y+"."+ext)
		if _, err := os.Stat(p); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("Content-Type", tileContentTypes[ext])
		http.ServeFile(w, r, p)
	})
}

// mapDatasetTransform is a Leaflet-style L.Transformation(a, b, c, d): tile-pixel(zoom) =
// 2^zoom * (a*worldX + b), 2^zoom * (c*worldY + d), where the canvas at native zoom 0 is
// TileSize square. Sourced verbatim from the tile provider's own config when available (e.g.
// THGL's cdn.th.gl/palworld/config/tiles.json) — fetch-map-tiles.sh never invents one.
type mapDatasetTransform struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
	C float64 `json:"c"`
	D float64 `json:"d"`
}

// mapDatasetLayer describes one tile pyramid within a (possibly multi-layer) map dataset, e.g.
// THGL's "default" (Palpagos) and "tree" (World Tree) layers each fetched into their own
// subdirectory of <dataDir>/map-tiles.
type mapDatasetLayer struct {
	ID        string               `json:"id"`
	Label     string               `json:"label,omitempty"`
	Path      string               `json:"path"`
	Format    string               `json:"format,omitempty"`
	TileSize  int                  `json:"tile_size,omitempty"`
	MinZoom   int                  `json:"min_zoom"`
	MaxZoom   int                  `json:"max_zoom"`
	Transform *mapDatasetTransform `json:"transform,omitempty"`
	Bounds    *[2][2]float64       `json:"bounds,omitempty"`
}

// mapDatasetInfo describes the provenance of the on-disk map tile pyramid. It is optionally
// recorded by whatever tool fetched the tiles (e.g. fetch-map-tiles.sh) as a sidecar file at
// <dataDir>/map-tiles/dataset.json, shaped like:
//
//	{"fetched_at": "2026-01-15T00:38:00Z", "game_version": "v1.0.0.100427", "source": "palworld.gg", "layers": []}
//
// fetched_at is an RFC3339 timestamp string (or null/absent if unknown); game_version and
// source are free-form strings the fetch tooling chooses to write. layers is empty for a
// legacy flat single-pyramid fetch (tiles live directly under map-tiles/{z}/{x}/{y}.ext); when
// non-empty, each entry's Path names the subdirectory under map-tiles/ holding that layer's
// pyramid. Palhelm only ever reads this file — it never writes dataset.json into the live data
// directory itself.
type mapDatasetInfo struct {
	FetchedAt   *string           `json:"fetched_at"`
	GameVersion string            `json:"game_version"`
	Source      string            `json:"source"`
	Notes       string            `json:"notes,omitempty"`
	Layers      []mapDatasetLayer `json:"layers"`
}

// preV1MapDataset is the conservative default reported when no dataset.json sidecar exists,
// which is the case for every tile pyramid fetched before the Palworld 1.0 release: better to
// tell the frontend "this predates 1.0" than to silently imply the tiles are current.
var preV1MapDataset = mapDatasetInfo{FetchedAt: nil, GameVersion: "pre-1.0", Source: "palworld.gg", Layers: []mapDatasetLayer{}}

// loadMapDataset reads the tile pyramid's provenance sidecar, defaulting to preV1MapDataset
// when absent or unparsable. Shared by the session GET /api/v1/map/dataset handler and the
// integration GET /map endpoint, which reshapes this into a dedicated, camelCase view
// struct rather than serializing it directly (spec §4).
func (s *Server) loadMapDataset() mapDatasetInfo {
	b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "map-tiles", "dataset.json"))
	if err != nil {
		return preV1MapDataset
	}
	var info mapDatasetInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return preV1MapDataset
	}
	if info.Layers == nil {
		info.Layers = []mapDatasetLayer{}
	}
	return info
}

// mapDataset serves GET /api/v1/map/dataset: the tile pyramid's fetch metadata, so the
// frontend can show a staleness badge when the tiles predate the running game version.
func (s *Server) mapDataset(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.loadMapDataset())
}
