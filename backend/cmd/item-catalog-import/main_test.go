package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/8tp/palhelm/internal/itemgrant"
)

func TestPrepareItemRequiresGrantableIconAndSafeURL(t *testing.T) {
	_, source, err := prepareItem(manifestItem{ID: "Stone", Name: "Stone", IconURL: "https://assets.example/stone.webp", MaxQuantity: 9999, Grantable: true})
	if err != nil || source == "" {
		t.Fatalf("prepare valid item source=%q err=%v", source, err)
	}
	if _, _, err := prepareItem(manifestItem{ID: "../Stone", Name: "Stone", IconURL: "https://assets.example/stone.webp", MaxQuantity: 1}); err == nil {
		t.Fatal("unsafe item id accepted")
	}
	if _, _, err := prepareItem(manifestItem{ID: "Stone", Name: "Stone", IconURL: "http://assets.example/stone.webp", MaxQuantity: 1}); err == nil {
		t.Fatal("non-HTTPS icon accepted")
	}
}

func TestValidateIconAndAllowlistOutput(t *testing.T) {
	dir := t.TempDir()
	icon := filepath.Join(dir, "stone.webp")
	if err := os.WriteFile(icon, []byte("RIFF0000WEBPpayload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateIcon(icon, ".webp"); err != nil {
		t.Fatal(err)
	}
	if err := validateIcon(icon, ".png"); err == nil {
		t.Fatal("WEBP accepted as PNG")
	}
	doc := catalogDocument{GameVersion: "1.0.1", Items: []itemgrant.Item{{ID: "Stone", MaxQuantity: 9999, Grantable: true}, {ID: "DebugOnly", MaxQuantity: 1, Grantable: false}}}
	allowlist := filepath.Join(dir, "allowlist.txt")
	if err := writeAllowlist(allowlist, doc); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(allowlist)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "# catalogItems=1\n") || !strings.Contains(text, "Stone\t9999\n") || strings.Contains(text, "DebugOnly") {
		t.Fatalf("allowlist = %q", text)
	}
}

func TestParsePalmodsEntriesAndSafetyPolicy(t *testing.T) {
	escaped := `\"entries\":[{\"id\":\"Wood\",\"name\":\"Wood\",\"hasGameIcon\":true,\"group\":\"Material\",\"canonical\":true,\"iconUrl\":\"https://assets.example/wood.webp\"}]},\"categories\"`
	entries, err := parsePalmodsEntries(escaped)
	if err != nil || len(entries) != 1 || entries[0].ID != "Wood" {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
	if maximum, allowed := palmodsQuantityPolicy("Material"); !allowed || maximum != 9999 {
		t.Fatalf("material policy=%d,%v", maximum, allowed)
	}
	for _, group := range []string{"Key Item", "Internal & other", "unknown"} {
		if _, allowed := palmodsQuantityPolicy(group); allowed {
			t.Fatalf("unsafe group %q allowed", group)
		}
	}
}

func TestPalmodsRaritiesFollowNumberedSchematicGrades(t *testing.T) {
	entries := []palmodsEntry{
		{ID: "CopperArmor", Name: "Metal Armor", Group: "Armor"},
		{ID: "CopperArmor_2", Name: "Metal Armor", Group: "Armor"},
		{ID: "CopperArmor_3", Name: "Metal Armor", Group: "Armor"},
		{ID: "CopperArmor_4", Name: "Metal Armor", Group: "Armor"},
		{ID: "CopperArmor_5", Name: "Metal Armor", Group: "Armor"},
		{ID: "Blueprint_CopperArmor_2", Name: "Metal Armor Schematic 1", Group: "Schematic"},
		{ID: "Blueprint_CopperArmor_3", Name: "Metal Armor Schematic 2", Group: "Schematic"},
		{ID: "Blueprint_CopperArmor_4", Name: "Metal Armor Schematic 3", Group: "Schematic"},
		{ID: "Blueprint_CopperArmor_5", Name: "Metal Armor Schematic 4", Group: "Schematic"},
		{ID: "DecalGun_2", Name: "Decal Gun 2", Group: "Weapon"},
		{ID: "Blueprint_Special", Name: "Special Schematic", Group: "Schematic"},
	}
	want := map[string]string{
		"copperarmor":             "Common",
		"copperarmor_2":           "Uncommon",
		"copperarmor_3":           "Rare",
		"copperarmor_4":           "Epic",
		"copperarmor_5":           "Legendary",
		"blueprint_copperarmor_2": "Uncommon",
		"blueprint_copperarmor_3": "Rare",
		"blueprint_copperarmor_4": "Epic",
		"blueprint_copperarmor_5": "Legendary",
	}
	got := palmodsRarities(entries)
	for id, rarity := range want {
		if got[id] != rarity {
			t.Errorf("rarity[%q] = %q, want %q", id, got[id], rarity)
		}
	}
	for _, id := range []string{"decalgun_2", "blueprint_special"} {
		if got[id] != "" {
			t.Errorf("ambiguous item %q received rarity %q", id, got[id])
		}
	}
}
