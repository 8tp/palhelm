package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPaldeckIconServesCaseInsensitively(t *testing.T) {
	dataDir := t.TempDir()
	iconsDir := filepath.Join(dataDir, "pal-icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconsDir, "sheepball.webp"), []byte("fake-webp-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)

	for _, id := range []string{"sheepball", "SheepBall", "SHEEPBALL"} {
		rr := request("GET", "/api/v1/paldeck/icon/"+id, "")
		if rr.Code != 200 {
			t.Fatalf("id=%q status=%d body=%s", id, rr.Code, rr.Body.String())
		}
		if rr.Body.String() != "fake-webp-bytes" {
			t.Fatalf("id=%q unexpected body %q", id, rr.Body.String())
		}
		if cc := rr.Header().Get("Cache-Control"); cc == "" {
			t.Fatalf("id=%q missing Cache-Control header", id)
		}
	}
}

func TestPaldeckIconPrefersWebpOverPng(t *testing.T) {
	dataDir := t.TempDir()
	iconsDir := filepath.Join(dataDir, "pal-icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconsDir, "anubis.webp"), []byte("webp-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconsDir, "anubis.png"), []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/api/v1/paldeck/icon/anubis", "")
	if rr.Code != 200 || rr.Body.String() != "webp-bytes" {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPaldeckIconUsesBaseSpeciesForSaveVariants(t *testing.T) {
	dataDir := t.TempDir()
	iconsDir := filepath.Join(dataDir, "pal-icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for id, body := range map[string]string{
		"grassmammoth.webp":     "mammorest",
		"plantslime.webp":       "gumoss",
		"boss_hunter_rifle.png": "hawk",
	} {
		if err := os.WriteFile(filepath.Join(iconsDir, id), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	request := newTestServerHandler(t, dataDir)
	for path, want := range map[string]string{
		"BOSS_GrassMammoth": "mammorest",
		"PlantSlime_Flower": "gumoss",
		"BOSS_Hunter_Rifle": "hawk",
	} {
		rr := request("GET", "/api/v1/paldeck/icon/"+path, "")
		if rr.Code != 200 || rr.Body.String() != want {
			t.Fatalf("id=%q status=%d body=%q want=%q", path, rr.Code, rr.Body.String(), want)
		}
	}
}

func TestPaldeckIconFallsBackToPng(t *testing.T) {
	dataDir := t.TempDir()
	iconsDir := filepath.Join(dataDir, "pal-icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconsDir, "anubis.png"), []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/api/v1/paldeck/icon/anubis", "")
	if rr.Code != 200 || rr.Body.String() != "png-bytes" {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPaldeckIconMissingIs404(t *testing.T) {
	request := newTestServerHandler(t, t.TempDir())
	rr := request("GET", "/api/v1/paldeck/icon/nonexistentpal", "")
	if rr.Code != 404 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPaldeckIconRejectsPathTraversal(t *testing.T) {
	dataDir := t.TempDir()
	// a file that a naive path join could otherwise escape the icons dir to reach
	if err := os.WriteFile(filepath.Join(dataDir, "secret.webp"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/api/v1/paldeck/icon/..%2Fsecret", "")
	if rr.Code != 404 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPaldeckIconDatasetDefaultsWhenSidecarMissing(t *testing.T) {
	request := newTestServerHandler(t, t.TempDir())
	rr := request("GET", "/api/v1/paldeck/icon-dataset", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Source       string   `json:"source"`
		FetchedAt    *string  `json:"fetchedAt"`
		Count        int      `json:"count"`
		CharacterIDs []string `json:"characterIds"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "unconfigured" || got.Count != 0 || got.FetchedAt != nil {
		t.Fatalf("got %#v", got)
	}
	if len(got.CharacterIDs) == 0 {
		t.Fatal("expected the full paldeck roster regardless of what's fetched on disk")
	}
}

func TestPaldeckIconDatasetReadsSidecarWhenPresent(t *testing.T) {
	dataDir := t.TempDir()
	iconsDir := filepath.Join(dataDir, "pal-icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sidecar := `{"source":"paldb.cc","fetched_at":"2026-07-10T12:00:00Z","count":231}`
	if err := os.WriteFile(filepath.Join(iconsDir, "dataset.json"), []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconsDir, "yakushimamonster001.png"), []byte("supplemental"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := newTestServerHandler(t, dataDir)
	rr := request("GET", "/api/v1/paldeck/icon-dataset", "")
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Source       string   `json:"source"`
		FetchedAt    *string  `json:"fetchedAt"`
		Count        int      `json:"count"`
		CharacterIDs []string `json:"characterIds"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "paldb.cc" || got.Count != 231 || got.FetchedAt == nil || *got.FetchedAt != "2026-07-10T12:00:00Z" {
		t.Fatalf("got %#v", got)
	}
	foundSupplemental := false
	for _, id := range got.CharacterIDs {
		if id == "yakushimamonster001" {
			foundSupplemental = true
			break
		}
	}
	if !foundSupplemental {
		t.Fatalf("supplemental installed icon missing from dataset: %#v", got.CharacterIDs)
	}
}
