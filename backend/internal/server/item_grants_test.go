package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8tp/palhelm/internal/config"
	"github.com/8tp/palhelm/internal/store"
)

func newItemGrantTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	iconDir := filepath.Join(dir, "icons")
	if err := os.MkdirAll(iconDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconDir, "stone.webp"), []byte("RIFF0000WEBPtest"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := `{"schemaVersion":1,"gameVersion":"1.0.1","source":{"name":"test export"},"items":[{"id":"Stone","name":"Stone","category":"Material","icon":"stone.webp","maxQuantity":9999,"grantable":true},{"id":"DebugOnly","name":"Debug Only","maxQuantity":1,"grantable":false}]}`
	catalogPath := filepath.Join(dir, "item-catalog.json")
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{AdminPassword: "panelpass", ViewerPassword: "viewerpass", SessionSecret: strings.Repeat("s", 48), DataDir: dir, ItemCatalogPath: catalogPath, ItemIconDir: iconDir, ItemGrantSpoolDir: filepath.Join(dir, "spool"), ItemGrantsEnabled: false, IntegrationRateLimit: 60}
	_, h := New(cfg, st, testLogger())
	return h, st
}

func TestItemCatalogIsViewerSafeAndGrantMutationIsFailClosed(t *testing.T) {
	h, st := newItemGrantTestServer(t)
	if err := st.UpsertLivePlayer(context.Background(), store.Player{UID: "abcdef12000000000000000000000000", Name: "Player"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	viewer := loginAs(t, h, "viewerpass")
	admin := loginAs(t, h, "panelpass")

	list := sessionRequest(h, http.MethodGet, "/api/v1/items?q=stone", "", viewer)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"Stone"`) || strings.Contains(list.Body.String(), "DebugOnly") {
		t.Fatalf("catalog status=%d body=%s", list.Code, list.Body.String())
	}
	icon := sessionRequest(h, http.MethodGet, "/api/v1/items/Stone/icon", "", viewer)
	if icon.Code != http.StatusOK || icon.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("icon status=%d type=%q", icon.Code, icon.Header().Get("Content-Type"))
	}
	capability := sessionRequest(h, http.MethodGet, "/api/v1/items/capability", "", viewer)
	if capability.Code != http.StatusOK || !strings.Contains(capability.Body.String(), `"ready":false`) || capability.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("capability status=%d headers=%v body=%s", capability.Code, capability.Header(), capability.Body.String())
	}
	body := `{"itemId":"Stone","quantity":10,"reason":"test grant","idempotencyKey":"test-key-123"}`
	forbidden := sessionRequest(h, http.MethodPost, "/api/v1/players/abcdef12/item-grants", body, viewer)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer mutation status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	unavailable := sessionRequest(h, http.MethodPost, "/api/v1/players/abcdef12/item-grants", body, admin)
	if unavailable.Code != http.StatusServiceUnavailable || !strings.Contains(unavailable.Body.String(), "item_grants_unavailable") {
		t.Fatalf("disabled mutation status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}
}
