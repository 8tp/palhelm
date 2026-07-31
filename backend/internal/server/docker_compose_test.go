package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/8tp/palhelm/internal/config"
)

func TestMapTilesInstallCommand(t *testing.T) {
	t.Run("uses explicit container name", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "docker-compose.yml")
		compose := "services:\n  dashboard:\n    container_name: palworld-dashboard\n"
		if err := os.WriteFile(path, []byte(compose), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{ComposeFile: path, PanelService: "dashboard"}
		if got, want := mapTilesInstallCommand(cfg), "docker exec palworld-dashboard palhelm fetch-map-tiles"; got != want {
			t.Fatalf("mapTilesInstallCommand() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to compose service", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "docker-compose.yml")
		compose := "services:\n  dashboard:\n    image: palhelm\n"
		if err := os.WriteFile(path, []byte(compose), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{ComposeFile: path, PanelService: "dashboard"}
		if got, want := mapTilesInstallCommand(cfg), "docker compose exec dashboard palhelm fetch-map-tiles"; got != want {
			t.Fatalf("mapTilesInstallCommand() = %q, want %q", got, want)
		}
	})

	t.Run("does not render compose interpolation as shell", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "docker-compose.yml")
		compose := "services:\n  palhelm:\n    container_name: ${PALHELM_CONTAINER_NAME:-palhelm}\n"
		if err := os.WriteFile(path, []byte(compose), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{ComposeFile: path}
		if got, want := mapTilesInstallCommand(cfg), "docker compose exec palhelm palhelm fetch-map-tiles"; got != want {
			t.Fatalf("mapTilesInstallCommand() = %q, want %q", got, want)
		}
	})
}

func TestPalIconsInstallCommand(t *testing.T) {
	t.Run("uses explicit container name", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "docker-compose.yml")
		compose := "services:\n  dashboard:\n    container_name: palworld-dashboard\n"
		if err := os.WriteFile(path, []byte(compose), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{ComposeFile: path, PanelService: "dashboard"}
		if got, want := palIconsInstallCommand(cfg), "docker exec palworld-dashboard palhelm fetch-pal-icons"; got != want {
			t.Fatalf("palIconsInstallCommand() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to compose service", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "docker-compose.yml")
		compose := "services:\n  dashboard:\n    image: palhelm\n"
		if err := os.WriteFile(path, []byte(compose), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{ComposeFile: path, PanelService: "dashboard"}
		if got, want := palIconsInstallCommand(cfg), "docker compose exec dashboard palhelm fetch-pal-icons"; got != want {
			t.Fatalf("palIconsInstallCommand() = %q, want %q", got, want)
		}
	})
}
