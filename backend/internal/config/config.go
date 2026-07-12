// Package config loads Palhelm's environment-only configuration.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config contains every runtime setting used by the backend.
type Config struct {
	Addr, DataDir, AdminPassword, ViewerPassword, SessionSecret string
	RESTURL, RESTUser, PalworldPassword, RCONAddr, SaveDir      string
	ComposeFile, GameService                                    string
	// SteamWebAPIKey is optional; when empty, player avatars resolve via Steam's
	// keyless public community endpoint instead of the Web API.
	SteamWebAPIKey                                     string
	TrustedProxies                                     []string
	DockerControl, SecureCookies                       bool
	MetricsInterval, PlayersInterval, SaveSyncInterval time.Duration
	IntegrationRateLimit                               int
}

// Load reads the environment and applies documented defaults.
func Load() (Config, error) {
	c := Config{
		Addr: env("PALHELM_ADDR", ":8080"), DataDir: env("PALHELM_DATA_DIR", "./data"),
		AdminPassword: os.Getenv("PALHELM_ADMIN_PASSWORD"), ViewerPassword: os.Getenv("PALHELM_VIEWER_PASSWORD"),
		RESTURL: os.Getenv("PALWORLD_REST_URL"), RESTUser: env("PALWORLD_REST_USER", "admin"),
		PalworldPassword: os.Getenv("PALWORLD_ADMIN_PASSWORD"), RCONAddr: os.Getenv("PALWORLD_RCON_ADDR"),
		SaveDir:     os.Getenv("PALWORLD_SAVE_DIR"),
		ComposeFile: os.Getenv("PALHELM_COMPOSE_FILE"), GameService: env("PALHELM_GAME_SERVICE", "palworld"),
		SteamWebAPIKey: strings.TrimSpace(os.Getenv("STEAM_WEB_API_KEY")),
	}
	var err error
	c.DockerControl = strings.EqualFold(os.Getenv("PALHELM_DOCKER_CONTROL"), "true")
	if raw := strings.TrimSpace(os.Getenv("PALHELM_TRUSTED_PROXIES")); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			cidr := strings.TrimSpace(value)
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return c, fmt.Errorf("PALHELM_TRUSTED_PROXIES contains invalid CIDR %q: %w", cidr, err)
			}
			c.TrustedProxies = append(c.TrustedProxies, cidr)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("PALHELM_SECURE_COOKIES")); raw != "" {
		if c.SecureCookies, err = strconv.ParseBool(raw); err != nil {
			return c, fmt.Errorf("PALHELM_SECURE_COOKIES: %w", err)
		}
	}
	if c.MetricsInterval, err = duration("PALHELM_METRICS_INTERVAL", 5*time.Second); err != nil {
		return c, err
	}
	if c.PlayersInterval, err = duration("PALHELM_PLAYERS_INTERVAL", 15*time.Second); err != nil {
		return c, err
	}
	if c.SaveSyncInterval, err = duration("PALHELM_SAVE_SYNC_INTERVAL", 10*time.Minute); err != nil {
		return c, err
	}
	if c.IntegrationRateLimit, err = positiveInt("PALHELM_INTEGRATION_RATE_LIMIT", 60); err != nil {
		return c, err
	}
	c.SessionSecret = os.Getenv("PALHELM_SESSION_SECRET")
	return c, nil
}

// ValidateServe verifies settings required by the serve command.
func (c Config) ValidateServe() error {
	if c.AdminPassword == "" {
		return errors.New("PALHELM_ADMIN_PASSWORD is required")
	}
	return nil
}

// EnsureSessionSecret generates and atomically persists a secret when none was configured.
func (c *Config) EnsureSessionSecret() error {
	if c.SessionSecret != "" {
		return nil
	}
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(c.DataDir, "session-secret")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		c.SessionSecret = string(b)
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	c.SessionSecret = base64.RawURLEncoding.EncodeToString(b)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(c.SessionSecret), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func duration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

// positiveInt parses an integer environment override that must be >= 1; a missing
// variable falls back silently, but an invalid or non-positive value fails startup
// (fail-closed, not a silent default — spec §8.1).
func positiveInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be an integer >= 1", key)
	}
	return n, nil
}
