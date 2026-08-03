package palgrant

import "testing"

func intPtr(v int) *int { return &v }

func TestCatalogExcludesHumansAndBossSaveVariants(t *testing.T) {
	c := NewCatalog()
	if c.Count() < 200 {
		t.Fatalf("catalogue unexpectedly small: %d", c.Count())
	}
	if _, ok := c.Species("Anubis"); !ok {
		t.Fatal("Anubis missing")
	}
	for _, id := range []string{"Hunter_Rifle", "BOSS_Hunter_Rifle"} {
		if _, ok := c.Species(id); ok {
			t.Fatalf("unsafe human/boss %q is grantable", id)
		}
	}
	if got := c.Search("anub", 10); len(got) != 1 || got[0].DisplayName != "Anubis" {
		t.Fatalf("search = %#v", got)
	}
}

func TestGenerationSpecValidation(t *testing.T) {
	natural := DefaultSpec()
	natural.LevelMin, natural.LevelMax = 20, 30
	if err := natural.Validate(); err != nil {
		t.Fatal(err)
	}
	custom := GenerationSpec{Mode: "custom", LevelMin: 35, LevelMax: 35, Gender: "female", Alpha: true, IVMode: "minimum", TalentHP: intPtr(70), TalentMelee: intPtr(70), TalentShot: intPtr(70), TalentDefense: intPtr(70), PassiveSkillIDs: []string{"CraftSpeed_up2"}, CondensationStars: 2}
	if err := custom.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, spec := range map[string]GenerationSpec{
		"alpha lucky":       {Mode: "custom", LevelMin: 1, LevelMax: 1, Gender: "random", Alpha: true, Lucky: true, IVMode: "natural"},
		"level overflow":    {Mode: "natural", LevelMin: 1, LevelMax: MaxSupportedLevel + 1, Gender: "random", IVMode: "natural"},
		"natural overrides": {Mode: "natural", LevelMin: 1, LevelMax: 1, Gender: "female", IVMode: "natural"},
		"missing exact IV":  {Mode: "custom", LevelMin: 1, LevelMax: 1, Gender: "random", IVMode: "exact", TalentHP: intPtr(50)},
	} {
		t.Run(name, func(t *testing.T) {
			if spec.PassiveSkillIDs == nil {
				spec.PassiveSkillIDs = []string{}
			}
			if err := spec.Validate(); err == nil {
				t.Fatal("invalid spec accepted")
			}
		})
	}
}
