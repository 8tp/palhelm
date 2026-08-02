package itemgrant

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestCatalog(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "items.json")
	doc := `{"schemaVersion":1,"gameVersion":"1.0.1","source":{"name":"test"},"items":[{"id":"Stone","name":"Stone","category":"Material","icon":"stone.webp","maxQuantity":9999,"grantable":true},{"id":"CopperArmor_5","name":"Metal Armor","category":"Armor","rarity":"Legendary","icon":"armor.webp","maxQuantity":10,"grantable":true},{"id":"DebugOnly","name":"Debug Only","maxQuantity":1,"grantable":false}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCatalogSearchAndDenyByDefault(t *testing.T) {
	catalog, err := LoadCatalog(writeTestCatalog(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	items, total := catalog.Search("material", 10, 0)
	if total != 1 || len(items) != 1 || items[0].ID != "Stone" {
		t.Fatalf("search = %#v total=%d", items, total)
	}
	items, total = catalog.Search("debug", 10, 0)
	if total != 0 || len(items) != 0 {
		t.Fatalf("non-grantable item leaked into search: %#v", items)
	}
	items, total = catalog.Search("legendary armor", 10, 0)
	if total != 1 || len(items) != 1 || items[0].ID != "CopperArmor_5" {
		t.Fatalf("rarity search = %#v total=%d", items, total)
	}
}

func TestSpoolCapabilityAndAtomicRequest(t *testing.T) {
	dir := t.TempDir()
	catalog, err := LoadCatalog(writeTestCatalog(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	spool := Spool{Dir: filepath.Join(dir, "spool")}
	if got := spool.Capability(true, catalog, time.Now()); got.Ready {
		t.Fatalf("missing bridge reported ready: %#v", got)
	}
	if err := os.MkdirAll(spool.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	heartbeat := `{"protocolVersion":1,"state":"ready","gameVersion":"1.0.1","catalogVersion":"1.0.1","catalogItems":2,"at":"` + now.Format(time.RFC3339) + `"}`
	if err := os.WriteFile(filepath.Join(spool.Dir, "capability.json"), []byte(heartbeat), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := spool.Capability(true, catalog, now); !got.Ready {
		t.Fatalf("valid bridge not ready: %#v", got)
	}
	req := Request{ProtocolVersion: 1, RequestID: "12345678", CreatedAt: now.Format(time.RFC3339), PlayerUID: "abcdef12", PlayerName: "Player", ItemID: "Stone", Quantity: 10}
	if err := spool.Submit(req); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(spool.Dir, "requests", "12345678.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(spool.Dir, "inbox.json")); err != nil {
		t.Fatal(err)
	}
	req.RequestID = "87654321"
	if err := spool.Submit(req); !errors.Is(err, ErrBridgeBusy) {
		t.Fatalf("duplicate submit error = %v, want ErrBridgeBusy", err)
	}
}
