package palgrant

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed data/passives.json
var passiveCatalogJSON []byte

type PassiveSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Rank        int    `json:"rank"`
	Inheritable bool   `json:"inheritable"`
}

type PassiveCatalogSource struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	URL     string `json:"url"`
}

type passiveCatalogFile struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	CatalogVersion string               `json:"catalogVersion"`
	Source         PassiveCatalogSource `json:"source"`
	Passives       []PassiveSkill       `json:"passives"`
}

type PassiveCatalog struct {
	version string
	source  PassiveCatalogSource
	items   []PassiveSkill
	byID    map[string]PassiveSkill
}

func NewPassiveCatalog() (*PassiveCatalog, error) {
	var file passiveCatalogFile
	if err := json.Unmarshal(passiveCatalogJSON, &file); err != nil {
		return nil, fmt.Errorf("decode embedded passive catalogue: %w", err)
	}
	if file.SchemaVersion != 1 || strings.TrimSpace(file.CatalogVersion) == "" || len(file.Passives) < 100 {
		return nil, fmt.Errorf("embedded passive catalogue is incomplete or incompatible")
	}
	c := &PassiveCatalog{version: file.CatalogVersion, source: file.Source, byID: make(map[string]PassiveSkill, len(file.Passives))}
	for _, item := range file.Passives {
		item.ID, item.Name, item.Description = strings.TrimSpace(item.ID), strings.TrimSpace(item.Name), strings.TrimSpace(item.Description)
		key := strings.ToLower(item.ID)
		if !safeToken.MatchString(item.ID) || item.Name == "" || c.byID[key].ID != "" {
			return nil, fmt.Errorf("embedded passive catalogue contains an invalid or duplicate ID")
		}
		c.byID[key] = item
		c.items = append(c.items, item)
	}
	return c, nil
}

func MustPassiveCatalog() *PassiveCatalog {
	c, err := NewPassiveCatalog()
	if err != nil {
		panic(err)
	}
	return c
}

func (c *PassiveCatalog) Count() int                   { return len(c.items) }
func (c *PassiveCatalog) Version() string              { return c.version }
func (c *PassiveCatalog) Source() PassiveCatalogSource { return c.source }
func (c *PassiveCatalog) Passive(id string) (PassiveSkill, bool) {
	item, ok := c.byID[strings.ToLower(strings.TrimSpace(id))]
	return item, ok
}

func (c *PassiveCatalog) Search(query string, limit int) []PassiveSkill {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	out := make([]PassiveSkill, 0, limit)
	for _, item := range c.items {
		haystack := strings.ToLower(item.Name + " " + item.ID + " " + item.Description)
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

func (c *PassiveCatalog) ValidateIDs(ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		key := strings.ToLower(strings.TrimSpace(id))
		if _, ok := c.byID[key]; !ok {
			return fmt.Errorf("passive skill %q is not in the pinned game catalogue", id)
		}
		if seen[key] {
			return fmt.Errorf("passive skill %q is duplicated", id)
		}
		seen[key] = true
	}
	return nil
}

func PassiveTier(rank int) string {
	switch {
	case rank >= 5:
		return "world_tree"
	case rank == 4:
		return "rainbow"
	case rank == 3:
		return "gold"
	case rank == 2:
		return "silver"
	case rank == 1:
		return "bronze"
	default:
		return "negative"
	}
}

// SortedRanks is useful to clients that want deterministic filter controls.
func (c *PassiveCatalog) SortedRanks() []int {
	set := make(map[int]bool)
	for _, item := range c.items {
		set[item.Rank] = true
	}
	out := make([]int, 0, len(set))
	for rank := range set {
		out = append(out, rank)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}
