package server

import (
	"os"
	"strings"

	"github.com/8tp/palhelm/internal/config"
	"gopkg.in/yaml.v3"
)

type composeContainerNames struct {
	Services map[string]struct {
		ContainerName string `yaml:"container_name"`
	} `yaml:"services"`
}

func mapTilesInstallCommand(cfg config.Config) string {
	return installCommand(cfg, "fetch-map-tiles")
}

func palIconsInstallCommand(cfg config.Config) string {
	return installCommand(cfg, "fetch-pal-icons")
}

func installCommand(cfg config.Config, command string) string {
	service := strings.TrimSpace(cfg.PanelService)
	if service == "" {
		service = "palhelm"
	}
	if b, err := os.ReadFile(cfg.ComposeFile); err == nil {
		var compose composeContainerNames
		if yaml.Unmarshal(b, &compose) == nil {
			name := strings.TrimSpace(compose.Services[service].ContainerName)
			if validDockerName(name) {
				return "docker exec " + name + " palhelm " + command
			}
		}
	}
	return "docker compose exec " + shellArg(service) + " palhelm " + command
}

func validDockerName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !asciiAlphaNumeric(r) {
			return false
		}
		if !asciiAlphaNumeric(r) && r != '_' && r != '.' && r != '-' {
			return false
		}
	}
	return true
}

func asciiAlphaNumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func shellArg(value string) string {
	if validDockerName(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
