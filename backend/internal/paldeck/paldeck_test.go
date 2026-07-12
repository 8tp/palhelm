package paldeck

import "testing"

func TestLookupKnownID(t *testing.T) {
	name, ok := Lookup("SheepBall")
	if !ok || name != "Lamball" {
		t.Fatalf("Lookup(SheepBall) = %q, %v; want Lamball, true", name, ok)
	}
}

func TestLookupKnownV1ID(t *testing.T) {
	name, ok := Lookup("BlueSkyDragon")
	if !ok || name != "Shaolong" {
		t.Fatalf("Lookup(BlueSkyDragon) = %q, %v; want Shaolong, true", name, ok)
	}
}

func TestLiveV1RawIDsResolveToLocalizedNames(t *testing.T) {
	tests := map[string]string{
		"Alpaca":            "Melpaca",
		"PurpleSpider":      "Tarantriss",
		"LeafMomonga":       "Herbil",
		"PinkRabbit_Grass":  "Ribbuny Botan",
		"PlantSlime_Flower": "Gumoss",
		"Hunter_Rifle":      "Syndicate Gunner",
	}
	for id, want := range tests {
		if got := Name(id); got != want {
			t.Errorf("Name(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestNamedBossNPCWinsBeforeBossPrefixFallback(t *testing.T) {
	if got := Name("BOSS_Hunter_Rifle"); got != "Hawk" {
		t.Fatalf("Name(BOSS_Hunter_Rifle) = %q, want Hawk", got)
	}
	if got := Name("Hunter_Rifle"); got != "Syndicate Gunner" {
		t.Fatalf("Name(Hunter_Rifle) = %q, want Syndicate Gunner", got)
	}
	if !IsBossID("BOSS_Hunter_Rifle") {
		t.Fatal("named boss NPC should retain boss/Alpha provenance")
	}
}

func TestLookupUnknownID(t *testing.T) {
	name, ok := Lookup("NotARealPal")
	if ok || name != "" {
		t.Fatalf("Lookup(NotARealPal) = %q, %v; want \"\", false", name, ok)
	}
}

func TestLookupCaseInsensitive(t *testing.T) {
	for _, id := range []string{"sheepball", "SHEEPBALL", "SheepBall", "sHeEpBaLl", "  SheepBall  "} {
		name, ok := Lookup(id)
		if !ok || name != "Lamball" {
			t.Errorf("Lookup(%q) = %q, %v; want Lamball, true", id, name, ok)
		}
	}
}

func TestBossIDsResolveToBaseSpeciesName(t *testing.T) {
	for _, id := range []string{"BOSS_GrassMammoth", "boss_grassmammoth", " BOSS_BOSS_GrassMammoth "} {
		name, ok := Lookup(id)
		if !ok || name != "Mammorest" {
			t.Errorf("Lookup(%q) = %q, %v; want Mammorest, true", id, name, ok)
		}
	}
	if got := Name("BOSS_SomeFuturePal"); got != "SomeFuturePal" {
		t.Errorf("Name(BOSS_SomeFuturePal) = %q, want prefix-free fallback", got)
	}
	if !IsBossID("BOSS_GrassMammoth") || IsBossID("GrassMammoth") {
		t.Error("boss prefix recognition is incorrect")
	}
	if got := BaseCharacterID("BOSS_GrassMammoth"); got != "GrassMammoth" {
		t.Errorf("BaseCharacterID = %q, want GrassMammoth", got)
	}
}

func TestNameFallsBackToRawID(t *testing.T) {
	if got := Name("SheepBall"); got != "Lamball" {
		t.Errorf("Name(SheepBall) = %q, want Lamball", got)
	}
	if got := Name("SomeFuturePal"); got != "SomeFuturePal" {
		t.Errorf("Name(SomeFuturePal) = %q, want unchanged raw id", got)
	}
}

func TestElementalVariantResolves(t *testing.T) {
	// A live roster surfaced the ice Foxparks variant as a raw ID.
	if got := Name("BOSS_Kitsunebi_Ice"); got != "Foxparks Cryst" {
		t.Errorf("Name(BOSS_Kitsunebi_Ice) = %q, want Foxparks Cryst", got)
	}
}

func TestGenericHumanNPCsResolveToArchetype(t *testing.T) {
	tests := map[string]string{
		"BOSS_Female_People03": "Human (Female)",
		"Male_People01":        "Human (Male)",
		"People05":             "Human",
	}
	for id, want := range tests {
		if got := Name(id); got != want {
			t.Errorf("Name(%q) = %q, want %q", id, got, want)
		}
	}
	// Named human bosses and Pals must not be swallowed by the generic-human rule.
	if got := Name("BOSS_Hunter_Rifle"); got != "Hawk" {
		t.Errorf("Name(BOSS_Hunter_Rifle) = %q, want Hawk (curated name must win)", got)
	}
	if got := Name("SomeFuturePal"); got != "SomeFuturePal" {
		t.Errorf("Name(SomeFuturePal) = %q, want unchanged raw id", got)
	}
}

// TestNoDuplicateKeysAcrossTables guards against the two maps silently disagreeing if a future
// data-drop edit introduces the same internal ID with two different display names.
func TestNoDuplicateKeysAcrossTables(t *testing.T) {
	for k, v1Name := range v1Names {
		if legacyName, ok := legacyNames[k]; ok && legacyName != v1Name {
			t.Errorf("key %q disagrees: legacyNames=%q v1Names=%q", k, legacyName, v1Name)
		}
	}
}
