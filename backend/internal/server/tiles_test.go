package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/palhelm/palhelm/internal/config"
	"github.com/palhelm/palhelm/internal/store"
)

func newTestServerHandler(t *testing.T, dataDir string) (h func(method, path, body string) *httptest.ResponseRecorder) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Config{DataDir: dataDir, AdminPassword: "panelpass", SessionSecret: strings.Repeat("s", 48), MetricsInterval: time.Hour, PlayersInterval: time.Hour, SaveSyncInterval: time.Hour}
	_, mux := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`{"password":"panelpass"}`))
	login.Header.Set("Content-Type", "application/json")
	lr := httptest.NewRecorder()
	mux.ServeHTTP(lr, login)
	if lr.Code != 200 {
		t.Fatalf("login=%d %s", lr.Code, lr.Body.String())
	}
	cookie := lr.Result().Cookies()[0]
	return func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}
}

func TestMapDatasetDefaultsToPreV1WhenSidecarMissing(t *testing.T) {
	request := newTestServerHandler(t, t.TempDir())
	rr := request("GET", "/api/v1/map/dataset", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got mapDatasetInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.GameVersion != "pre-1.0" || got.Source != "palworld.gg" || got.FetchedAt != nil {
		t.Fatalf("got %#v", got)
	}
}

func TestMapDatasetReadsSidecarWhenPresent(t *testing.T) {
	dataDir := t.TempDir()
	tilesDir := filepath.Join(dataDir, "map-tiles")
	if err := os.MkdirAll(tilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sidecar := `{"fetched_at":"2026-07-01T00:38:00Z","game_version":"v1.0.0.100427","source":"palworld.th.gl"}`
	if err := os.WriteFile(filepath.Join(tilesDir, "dataset.json"), []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/api/v1/map/dataset", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got mapDatasetInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.GameVersion != "v1.0.0.100427" || got.Source != "palworld.th.gl" || got.FetchedAt == nil || *got.FetchedAt != "2026-07-01T00:38:00Z" {
		t.Fatalf("got %#v", got)
	}
	var raw map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if layers, ok := raw["layers"].([]any); !ok || len(layers) != 0 {
		t.Fatalf("legacy sidecar layers = %#v, want []", raw["layers"])
	}
}

func TestMapDatasetReadsLayeredSidecarWithTransform(t *testing.T) {
	dataDir := t.TempDir()
	tilesDir := filepath.Join(dataDir, "map-tiles")
	if err := os.MkdirAll(tilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sidecar := `{
		"source": "thgl",
		"fetched_at": "2026-07-10T12:59:00Z",
		"game_version": "1.0",
		"notes": "Palpagos offset still needs fixing upstream.",
		"layers": [
			{
				"id": "default",
				"label": "Palpagos",
				"path": "default",
				"format": "webp",
				"tile_size": 512,
				"min_zoom": 0,
				"max_zoom": 4,
				"transform": {"a": 0.000353395913859746, "b": 256, "c": -0.000353395913859746, "d": 123.47653230259525},
				"bounds": [[-1099399, -724399], [349399, 724399]]
			},
			{
				"id": "tree",
				"label": "World Tree",
				"path": "tree",
				"format": "webp",
				"tile_size": 512,
				"min_zoom": 0,
				"max_zoom": 4
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(tilesDir, "dataset.json"), []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/api/v1/map/dataset", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got mapDatasetInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Notes == "" || len(got.Layers) != 2 {
		t.Fatalf("got %#v", got)
	}
	def := got.Layers[0]
	if def.ID != "default" || def.Label != "Palpagos" || def.Format != "webp" || def.TileSize != 512 || def.MaxZoom != 4 {
		t.Fatalf("default layer=%#v", def)
	}
	if def.Transform == nil || def.Transform.A != 0.000353395913859746 || def.Transform.B != 256 {
		t.Fatalf("default layer transform=%#v", def.Transform)
	}
	if def.Bounds == nil || def.Bounds[0][0] != -1099399 || def.Bounds[1][1] != 724399 {
		t.Fatalf("default layer bounds=%#v", def.Bounds)
	}
	tree := got.Layers[1]
	if tree.ID != "tree" || tree.Transform != nil {
		t.Fatalf("tree layer=%#v", tree)
	}
}

func TestTilesServesLegacyFlatPng(t *testing.T) {
	dataDir := t.TempDir()
	tilePath := filepath.Join(dataDir, "map-tiles", "0", "0", "0.png")
	if err := os.MkdirAll(filepath.Dir(tilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tilePath, []byte("fake-png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/map-tiles/0/0/0.png", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type=%q", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "private,") {
		t.Fatalf("cache-control=%q", cc)
	}
	if rr.Body.String() != "fake-png-bytes" {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestTilesServesWebp(t *testing.T) {
	dataDir := t.TempDir()
	tilePath := filepath.Join(dataDir, "map-tiles", "0", "0", "0.webp")
	if err := os.MkdirAll(filepath.Dir(tilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tilePath, []byte("fake-webp-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/map-tiles/0/0/0.webp", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/webp" {
		t.Fatalf("content-type=%q", ct)
	}
	if rr.Body.String() != "fake-webp-bytes" {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestTilesServesLayeredPath(t *testing.T) {
	dataDir := t.TempDir()
	tilePath := filepath.Join(dataDir, "map-tiles", "default", "2", "1", "3.webp")
	if err := os.MkdirAll(filepath.Dir(tilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tilePath, []byte("layered-tile"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a sibling legacy-shaped path must NOT resolve to the layered file
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/map-tiles/default/2/1/3.webp", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "layered-tile" {
		t.Fatalf("body=%q", rr.Body.String())
	}
	miss := request("GET", "/map-tiles/2/1/3.webp", "")
	if miss.Code != 404 {
		t.Fatalf("expected 404 for unlayered path, got %d", miss.Code)
	}
}

func TestTilesRejectsPathTraversalInLayerSegment(t *testing.T) {
	dataDir := t.TempDir()
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/map-tiles/../../etc/0/0/0.png", "")
	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}
