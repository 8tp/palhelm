package sav

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// oodleLibAvailable reports whether the proprietary Oodle runtime is present
// locally. It is never redistributed with the source, so on a clean checkout
// (and in CI) it is absent and the tests that decompress real .sav fixtures
// skip instead of failing. Set it up in TestMain by pointing PALHELM_OODLE_LIB
// at the on-disk copy when it exists.
var oodleLibAvailable bool

func TestMain(m *testing.M) {
	lib, err := filepath.Abs(filepath.Join("..", "..", "data", oodleLibrary))
	if err != nil {
		panic(err)
	}
	if _, statErr := os.Stat(lib); statErr == nil {
		_ = os.Setenv("PALHELM_OODLE_LIB", lib)
		oodleLibAvailable = true
	}
	code := m.Run()
	os.Exit(code)
}

// requireOodle skips a test when the Oodle runtime is unavailable, so that
// clones without the proprietary library (including CI) stay green.
func requireOodle(t *testing.T) {
	t.Helper()
	if !oodleLibAvailable {
		t.Skip("Oodle runtime not present; set up backend/data/" + oodleLibrary + " to run save-decompression tests")
	}
}

func TestLevelOodleGroundTruth(t *testing.T) {
	requireOodle(t)
	savBytes, err := os.ReadFile(filepath.Join("testdata", "Level.sav"))
	if err != nil {
		t.Fatal(err)
	}
	got, h, err := readContainer(savBytes)
	if err != nil {
		t.Fatal(err)
	}
	if h.Magic != "PlM" {
		t.Fatalf("magic %q", h.Magic)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "Level.gvas"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 278289 {
		t.Fatalf("decompressed length %d", len(got))
	}
	if !bytes.Equal(got, want) {
		t.Fatal("Oodle output differs from Level.gvas")
	}
}

func TestParseLevel(t *testing.T) {
	requireOodle(t)
	w, err := ParseLevel(filepath.Join("testdata", "Level.sav"), Options{PlayersDir: filepath.Join("testdata", "does-not-exist")})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Guilds) != 7 {
		t.Fatalf("guild-map entries %d, want 7 (failures: %#v)", len(w.Guilds), w.Stats.DecodeFailures)
	}
	if len(w.Players) != 0 {
		t.Fatalf("players %d, want 0", len(w.Players))
	}
	if w.Meta.WorldName != "Autosave_W" {
		t.Fatalf("level meta world name %q", w.Meta.WorldName)
	}
}

func TestParseLevelMeta(t *testing.T) {
	requireOodle(t)
	m, err := ParseLevelMeta(filepath.Join("testdata", "LevelMeta.sav"))
	if err != nil {
		t.Fatal(err)
	}
	if m.WorldName != "Autosave_W" {
		t.Fatalf("world name %q, want verified fixture value %q", m.WorldName, "Autosave_W")
	}
	if m.Day != 1 {
		t.Fatalf("day %d, want 1", m.Day)
	}
}

func BenchmarkParseLevel(b *testing.B) {
	if !oodleLibAvailable {
		b.Skip("Oodle runtime not present; set up backend/data/" + oodleLibrary + " to run this benchmark")
	}
	path := filepath.Join("testdata", "Level.sav")
	// Set-up decompression once so the benchmark measures steady-state parsing.
	if _, err := ParseLevel(path, Options{PlayersDir: filepath.Join("testdata", "missing")}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := ParseLevel(path, Options{PlayersDir: filepath.Join("testdata", "missing")}); err != nil {
			b.Fatal(err)
		}
	}
}
