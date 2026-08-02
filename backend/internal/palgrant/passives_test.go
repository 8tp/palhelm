package palgrant

import "testing"

func TestPassiveCatalogContainsVersionPinnedRainbowAndWorldTreeTraits(t *testing.T) {
	c := MustPassiveCatalog()
	if c.Count() != 115 {
		t.Fatalf("passive catalogue count = %d, want 115", c.Count())
	}
	for _, test := range []struct {
		id   string
		name string
		tier string
	}{
		{"CraftSpeed_up3", "Remarkable Craftsmanship", "rainbow"},
		{"PAL_ALLAttack_up3", "Demon God", "rainbow"},
		{"WorkSuitabilityAddRank_MonsterFarm_2", "Ranch Master", "rainbow"},
		{"WorldTree_ATK", "Twin-Edged Holy Blade", "world_tree"},
	} {
		item, ok := c.Passive(test.id)
		if !ok || item.Name != test.name || PassiveTier(item.Rank) != test.tier {
			t.Fatalf("passive %q = %#v, found=%v", test.id, item, ok)
		}
	}
	if c.Version() != "palcalc_v23_b5e13e90fedc" {
		t.Fatalf("version = %q", c.Version())
	}
}

func TestPassiveCatalogValidationRejectsUnknownAndDuplicateIDs(t *testing.T) {
	c := MustPassiveCatalog()
	if err := c.ValidateIDs([]string{"CraftSpeed_up3", "PAL_ALLAttack_up3"}); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{"NotARealPassive"}, {"CraftSpeed_up3", "craftspeed_UP3"}} {
		if err := c.ValidateIDs(ids); err == nil {
			t.Fatalf("invalid passive IDs accepted: %#v", ids)
		}
	}
}

func TestPassiveSearchIncludesDescriptions(t *testing.T) {
	c := MustPassiveCatalog()
	got := c.Search("work speed +75", 10)
	if len(got) != 1 || got[0].ID != "CraftSpeed_up3" {
		t.Fatalf("search result = %#v", got)
	}
}
