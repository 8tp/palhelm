package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/8tp/palhelm/internal/paldeck"
)

// palIconExtensions are tried in order when resolving a CharacterID to an on-disk icon file.
// scripts/fetch-pal-icons.sh currently writes .webp (its source, paldb.cc, only serves icons in
// that format); .png is also accepted so an operator can drop in hand-converted or
// palworld.gg-sourced PNGs without any backend change.
var palIconExtensions = []string{".webp", ".png"}

// paldeckIconDir resolves the operator-fetched pal-icon set, matching the pattern tilesHandler
// uses for map-tiles: a fixed subdirectory of the configured data dir, populated out-of-band (by
// scripts/fetch-pal-icons.sh) rather than bundled with the binary, because pal icons are
// Pocketpair-derived art.
func (s *Server) paldeckIconDir() string {
	return filepath.Join(s.cfg.DataDir, "pal-icons")
}

// paldeckIcon serves GET /api/v1/paldeck/icon/{characterId}: the operator-fetched preview icon
// for a Pal CharacterID, resolved case-insensitively (save data does not reliably preserve a
// CharacterID's original casing, and neither does the URL a frontend caller constructs from it).
// 404 means "not installed for this id" — the frontend is expected to fall back to an initials
// avatar, the same contract tilesHandler uses for "tiles not installed".
func (s *Server) paldeckIcon(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "characterId")))
	if id == "" || strings.ContainsAny(id, "/\\") {
		http.NotFound(w, r)
		return
	}
	dir := s.paldeckIconDir()
	for _, ext := range palIconExtensions {
		p := filepath.Join(dir, id+ext)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
			http.ServeFile(w, r, p)
			return
		}
	}
	http.NotFound(w, r)
}

// palIconDatasetInfo mirrors mapDatasetInfo's shape for the pal-icon set's provenance sidecar,
// written by scripts/fetch-pal-icons.sh at <dataDir>/pal-icons/dataset.json:
//
//	{"source": "paldb.cc", "fetched_at": "2026-07-10T12:00:00Z", "count": 231}
//
// Palhelm only ever reads this file.
type palIconDatasetInfo struct {
	Source    string  `json:"source"`
	FetchedAt *string `json:"fetched_at"`
	Count     int     `json:"count"`
}

// paldeckIconDataset serves GET /api/v1/paldeck/icon-dataset: the installed icon set's fetch
// metadata (source/fetched_at/count), plus the full CharacterID roster so the frontend can know
// up front which ids it should even bother requesting an icon for, without 245 speculative GETs.
func (s *Server) paldeckIconDataset(w http.ResponseWriter, r *http.Request) {
	info := palIconDatasetInfo{Source: "unconfigured", Count: 0}
	if b, err := os.ReadFile(filepath.Join(s.paldeckIconDir(), "dataset.json")); err == nil {
		_ = json.Unmarshal(b, &info)
	}
	entries := paldeck.All()
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	writeJSON(w, 200, map[string]any{"source": info.Source, "fetchedAt": info.FetchedAt, "count": info.Count, "characterIds": ids})
}
