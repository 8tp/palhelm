package main

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestFetchMapTiles(t *testing.T) {
	original := fetchMapTilesCommand
	t.Cleanup(func() { fetchMapTilesCommand = original })

	t.Run("uses data directory as default destination", func(t *testing.T) {
		t.Setenv("PALHELM_DATA_DIR", "/custom/data")
		var got []string
		fetchMapTilesCommand = func(args ...string) *exec.Cmd {
			got = append([]string(nil), args...)
			return exec.Command("sh", "-c", "exit 0")
		}

		if err := fetchMapTiles(nil); err != nil {
			t.Fatalf("fetchMapTiles() error = %v", err)
		}
		want := []string{"/custom/data/map-tiles"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fetchMapTiles() args = %q, want %q", got, want)
		}
	})

	t.Run("forwards explicit arguments", func(t *testing.T) {
		var got []string
		fetchMapTilesCommand = func(args ...string) *exec.Cmd {
			got = append([]string(nil), args...)
			return exec.Command("sh", "-c", "exit 0")
		}
		want := []string{"--dest", "/tiles", "--force"}

		if err := fetchMapTiles(want); err != nil {
			t.Fatalf("fetchMapTiles() error = %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fetchMapTiles() args = %q, want %q", got, want)
		}
	})

	t.Run("returns downloader failure", func(t *testing.T) {
		fetchMapTilesCommand = func(args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 17")
		}

		err := fetchMapTiles(nil)
		if err == nil || !strings.Contains(err.Error(), "fetch map tiles: exit status 17") {
			t.Fatalf("fetchMapTiles() error = %v", err)
		}
	})
}
