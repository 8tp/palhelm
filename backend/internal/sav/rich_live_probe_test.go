package sav

import (
	"os"
	"testing"
)

// TestLiveRichPalProbe validates aggregate field coverage against a read-only
// live save without logging player names, UIDs, Pal names, or individual stats.
func TestLiveRichPalProbe(t *testing.T) {
	path := os.Getenv("PROBE_RICH_SAV")
	if path == "" {
		t.Skip("set PROBE_RICH_SAV=/path/to/Level.sav to run the rich-Pal probe")
	}
	world, err := ParseLevel(path, Options{})
	if err != nil {
		t.Fatalf("ParseLevel: %v", err)
	}
	gender, talents, passives, equipped, hp := 0, 0, 0, 0, 0
	for _, pal := range world.Pals {
		if pal.Gender != "" {
			gender++
		}
		if len(pal.Talents) > 0 {
			talents++
		}
		if len(pal.PassiveSkillIDs) > 0 {
			passives++
		}
		if len(pal.EquippedSkillIDs) > 0 {
			equipped++
		}
		if pal.HP >= 0 {
			hp++
		}
	}
	t.Logf("rich pal coverage: total=%d hp=%d gender=%d talents=%d passives=%d equipped=%d", len(world.Pals), hp, gender, talents, passives, equipped)
	if len(world.Pals) == 0 || gender == 0 || talents == 0 || passives == 0 || equipped == 0 {
		t.Fatalf("rich Pal extraction has implausibly empty coverage")
	}
}
