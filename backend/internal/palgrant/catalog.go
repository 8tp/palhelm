// Package palgrant owns the fail-closed contract for creating one legitimate Pal
// and granting it to an online player. It intentionally does not edit save files.
package palgrant

import (
	"errors"
	"regexp"
	"sort"
	"strings"

	"github.com/8tp/palhelm/internal/paldeck"
)

const CatalogVersion = "palworld_1.0_pinned"
const MaxSupportedLevel = 80

var safeToken = regexp.MustCompile(`^[A-Za-z0-9_]{1,160}$`)

type Species struct {
	CharacterID string `json:"characterId"`
	DisplayName string `json:"displayName"`
}

type Catalog struct {
	items []Species
	byID  map[string]Species
}

func NewCatalog() *Catalog {
	c := &Catalog{byID: make(map[string]Species)}
	for _, entry := range paldeck.All() {
		id := strings.TrimSpace(entry.ID)
		// Capturable humans and BOSS_ save variants are intentionally not a
		// creation catalogue. Alpha is represented as a validated specimen flag.
		if !safeToken.MatchString(id) || strings.HasPrefix(strings.ToLower(id), "boss_") || strings.EqualFold(id, "hunter_rifle") {
			continue
		}
		item := Species{CharacterID: id, DisplayName: strings.TrimSpace(entry.Name)}
		if item.DisplayName == "" {
			continue
		}
		c.byID[strings.ToLower(id)] = item
		c.items = append(c.items, item)
	}
	sort.Slice(c.items, func(i, j int) bool {
		return strings.ToLower(c.items[i].DisplayName) < strings.ToLower(c.items[j].DisplayName)
	})
	return c
}

func (c *Catalog) Count() int { return len(c.items) }

func (c *Catalog) Species(characterID string) (Species, bool) {
	item, ok := c.byID[strings.ToLower(strings.TrimSpace(characterID))]
	return item, ok
}

func (c *Catalog) Search(query string, limit int) []Species {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	out := make([]Species, 0, limit)
	for _, item := range c.items {
		haystack := strings.ToLower(item.DisplayName + " " + item.CharacterID)
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, item)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

type GenerationSpec struct {
	Mode              string   `json:"mode"`
	LevelMin          int      `json:"levelMin"`
	LevelMax          int      `json:"levelMax"`
	Gender            string   `json:"gender"`
	Alpha             bool     `json:"alpha"`
	Lucky             bool     `json:"lucky"`
	IVMode            string   `json:"ivMode"`
	TalentHP          *int     `json:"talentHp,omitempty"`
	TalentMelee       *int     `json:"talentMelee,omitempty"`
	TalentShot        *int     `json:"talentShot,omitempty"`
	TalentDefense     *int     `json:"talentDefense,omitempty"`
	PassiveSkillIDs   []string `json:"passiveSkillIds"`
	CondensationStars int      `json:"condensationStars"`
}

func DefaultSpec() GenerationSpec {
	return GenerationSpec{Mode: "natural", LevelMin: 1, LevelMax: 1, Gender: "random", IVMode: "natural", PassiveSkillIDs: []string{}}
}

func (s GenerationSpec) Validate() error {
	if s.Mode != "natural" && s.Mode != "custom" {
		return errors.New("mode must be natural or custom")
	}
	if s.LevelMin < 1 || s.LevelMax < s.LevelMin || s.LevelMax > MaxSupportedLevel {
		return errors.New("level range is outside the supported game range")
	}
	if s.Gender != "random" && s.Gender != "male" && s.Gender != "female" {
		return errors.New("gender must be random, male, or female")
	}
	if s.Alpha && s.Lucky {
		return errors.New("alpha and lucky cannot both be forced")
	}
	if s.IVMode != "natural" && s.IVMode != "minimum" && s.IVMode != "exact" {
		return errors.New("ivMode must be natural, minimum, or exact")
	}
	values := []*int{s.TalentHP, s.TalentMelee, s.TalentShot, s.TalentDefense}
	if s.IVMode == "natural" {
		for _, value := range values {
			if value != nil {
				return errors.New("natural IV mode cannot include IV overrides")
			}
		}
	} else {
		for _, value := range values {
			if value == nil || *value < 0 || *value > 100 {
				return errors.New("custom IV values must each be from 0 to 100")
			}
		}
	}
	if s.CondensationStars < 0 || s.CondensationStars > 4 {
		return errors.New("condensationStars must be from 0 to 4")
	}
	if len(s.PassiveSkillIDs) > 4 {
		return errors.New("a Pal cannot have more than four passive skills")
	}
	seen := make(map[string]bool, len(s.PassiveSkillIDs))
	for _, id := range s.PassiveSkillIDs {
		if !safeToken.MatchString(id) || seen[strings.ToLower(id)] {
			return errors.New("passive skill IDs must be unique safe game IDs")
		}
		seen[strings.ToLower(id)] = true
	}
	if s.Mode == "natural" && (s.Gender != "random" || s.Alpha || s.Lucky || s.IVMode != "natural" || len(s.PassiveSkillIDs) > 0 || s.CondensationStars != 0) {
		return errors.New("natural mode cannot include advanced overrides")
	}
	return nil
}
