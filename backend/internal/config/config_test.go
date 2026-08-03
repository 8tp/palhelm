package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTrustedProxyAndSecureCookieSettings(t *testing.T) {
	t.Setenv("PALHELM_TRUSTED_PROXIES", "10.0.0.0/8, 192.0.2.10/32")
	t.Setenv("PALHELM_SECURE_COOKIES", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SecureCookies || len(cfg.TrustedProxies) != 2 {
		t.Fatalf("config=%#v", cfg)
	}
}

func TestLoadRejectsInvalidTrustedProxy(t *testing.T) {
	t.Setenv("PALHELM_TRUSTED_PROXIES", "not-a-cidr")
	t.Setenv("PALHELM_SECURE_COOKIES", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid CIDR error")
	}
}

func TestLoadRejectsInvalidSecureCookieSetting(t *testing.T) {
	t.Setenv("PALHELM_TRUSTED_PROXIES", "")
	t.Setenv("PALHELM_SECURE_COOKIES", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid boolean error")
	}
}

func TestLoadDefaultsIntegrationRateLimit(t *testing.T) {
	t.Setenv("PALHELM_INTEGRATION_RATE_LIMIT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntegrationRateLimit != 60 {
		t.Fatalf("IntegrationRateLimit = %d, want default 60", cfg.IntegrationRateLimit)
	}
}

func TestLoadAcceptsIntegrationRateLimitOverride(t *testing.T) {
	t.Setenv("PALHELM_INTEGRATION_RATE_LIMIT", "120")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntegrationRateLimit != 120 {
		t.Fatalf("IntegrationRateLimit = %d, want 120", cfg.IntegrationRateLimit)
	}
}

// TestLoadRejectsInvalidIntegrationRateLimit proves spec §8.1: an invalid override fails
// startup closed rather than silently falling back to the default.
func TestLoadRejectsInvalidIntegrationRateLimit(t *testing.T) {
	for _, bad := range []string{"0", "-1", "not-a-number", "1.5"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("PALHELM_INTEGRATION_RATE_LIMIT", bad)
			if _, err := Load(); err == nil {
				t.Fatalf("PALHELM_INTEGRATION_RATE_LIMIT=%q did not fail startup", bad)
			}
		})
	}
}

func TestLoadGameDataDefaultsDisabled(t *testing.T) {
	t.Setenv("PALHELM_GAME_DATA_ENABLED", "")
	t.Setenv("PALHELM_GAME_DATA_INTERVAL", "")
	t.Setenv("PALHELM_GAME_DATA_TIMEOUT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GameDataEnabled || cfg.GameDataInterval != 30*time.Second || cfg.GameDataTimeout != 10*time.Second {
		t.Fatalf("game-data defaults = enabled:%v interval:%s timeout:%s", cfg.GameDataEnabled, cfg.GameDataInterval, cfg.GameDataTimeout)
	}
}

func TestLoadItemGrantsDefaultsFailClosed(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("PALHELM_DATA_DIR", dataDir)
	t.Setenv("PALHELM_ITEM_GRANTS_ENABLED", "")
	t.Setenv("PALHELM_ITEM_CATALOG_PATH", "")
	t.Setenv("PALHELM_ITEM_ICON_DIR", "")
	t.Setenv("PALHELM_ITEM_GRANT_SPOOL_DIR", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ItemGrantsEnabled {
		t.Fatal("item grants defaulted enabled")
	}
	if cfg.ItemCatalogPath != filepath.Join(dataDir, "item-catalog.json") || cfg.ItemIconDir != filepath.Join(dataDir, "item-icons") || cfg.ItemGrantSpoolDir != filepath.Join(dataDir, "item-grants") {
		t.Fatalf("item grant paths = %#v", cfg)
	}
}

func TestLoadItemGrantOverrides(t *testing.T) {
	t.Setenv("PALHELM_ITEM_GRANTS_ENABLED", "true")
	t.Setenv("PALHELM_ITEM_CATALOG_PATH", "/catalog/items.json")
	t.Setenv("PALHELM_ITEM_ICON_DIR", "/catalog/icons")
	t.Setenv("PALHELM_ITEM_GRANT_SPOOL_DIR", "/bridge/spool")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ItemGrantsEnabled || cfg.ItemCatalogPath != "/catalog/items.json" || cfg.ItemIconDir != "/catalog/icons" || cfg.ItemGrantSpoolDir != "/bridge/spool" {
		t.Fatalf("item grant overrides = %#v", cfg)
	}
}

func TestLoadRejectsInvalidItemGrantBoolean(t *testing.T) {
	t.Setenv("PALHELM_ITEM_GRANTS_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid item grant boolean to fail")
	}
}

func TestLoadPalGrantsDefaultFailClosedAndAcceptOverrides(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("PALHELM_DATA_DIR", dataDir)
	t.Setenv("PALHELM_PAL_GRANTS_ENABLED", "")
	t.Setenv("PALHELM_PAL_GRANT_SPOOL_DIR", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PalGrantsEnabled || cfg.PalGrantSpoolDir != filepath.Join(dataDir, "pal-grants") {
		t.Fatalf("Pal grant defaults = %#v", cfg)
	}
	t.Setenv("PALHELM_PAL_GRANTS_ENABLED", "true")
	t.Setenv("PALHELM_PAL_GRANT_SPOOL_DIR", "/bridge/pals")
	cfg, err = Load()
	if err != nil || !cfg.PalGrantsEnabled || cfg.PalGrantSpoolDir != "/bridge/pals" {
		t.Fatalf("Pal grant overrides = %#v err=%v", cfg, err)
	}
	t.Setenv("PALHELM_PAL_GRANTS_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("invalid Pal grant boolean did not fail startup")
	}
}

func TestLoadGameDataOverrides(t *testing.T) {
	t.Setenv("PALHELM_GAME_DATA_ENABLED", "true")
	t.Setenv("PALHELM_GAME_DATA_INTERVAL", "45s")
	t.Setenv("PALHELM_GAME_DATA_TIMEOUT", "12s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GameDataEnabled || cfg.GameDataInterval != 45*time.Second || cfg.GameDataTimeout != 12*time.Second {
		t.Fatalf("game-data overrides = %#v", cfg)
	}
}

func TestLoadRejectsUnsafeGameDataConfig(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"PALHELM_GAME_DATA_ENABLED", "maybe"},
		{"PALHELM_GAME_DATA_INTERVAL", "5s"},
		{"PALHELM_GAME_DATA_INTERVAL", "forever"},
		{"PALHELM_GAME_DATA_TIMEOUT", "0s"},
		{"PALHELM_GAME_DATA_TIMEOUT", "1m"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv("PALHELM_GAME_DATA_ENABLED", "")
			t.Setenv("PALHELM_GAME_DATA_INTERVAL", "")
			t.Setenv("PALHELM_GAME_DATA_TIMEOUT", "")
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid game-data config to fail")
			}
		})
	}
}
