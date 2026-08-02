// Package itemgrant implements Palhelm's fail-closed item mutation boundary.
package itemgrant

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const CatalogSchemaVersion = 1

var safeID = regexp.MustCompile(`^[A-Za-z0-9_]{1,160}$`)

type CatalogSource struct {
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	GeneratedAt string `json:"generatedAt,omitempty"`
}

type Item struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Rarity      string `json:"rarity,omitempty"`
	Icon        string `json:"icon,omitempty"`
	MaxQuantity int    `json:"maxQuantity"`
	Grantable   bool   `json:"grantable"`
}

type catalogDocument struct {
	SchemaVersion int           `json:"schemaVersion"`
	GameVersion   string        `json:"gameVersion"`
	Source        CatalogSource `json:"source"`
	Items         []Item        `json:"items"`
}

type Catalog struct {
	gameVersion string
	source      CatalogSource
	items       []Item
	byID        map[string]Item
	grantable   int
}

func LoadCatalog(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 32<<20 {
		return nil, errors.New("item catalogue exceeds 32 MiB")
	}
	var doc catalogDocument
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode item catalogue: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("item catalogue contains trailing data")
	}
	if doc.SchemaVersion != CatalogSchemaVersion {
		return nil, fmt.Errorf("unsupported item catalogue schema %d", doc.SchemaVersion)
	}
	if strings.TrimSpace(doc.GameVersion) == "" {
		return nil, errors.New("item catalogue gameVersion is required")
	}
	if strings.TrimSpace(doc.Source.Name) == "" {
		return nil, errors.New("item catalogue source.name is required")
	}
	if len(doc.Items) == 0 || len(doc.Items) > 10000 {
		return nil, fmt.Errorf("item catalogue must contain 1 to 10000 items")
	}
	c := &Catalog{gameVersion: strings.TrimSpace(doc.GameVersion), source: doc.Source, byID: make(map[string]Item, len(doc.Items))}
	for i, item := range doc.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Icon = strings.TrimSpace(item.Icon)
		if !safeID.MatchString(item.ID) {
			return nil, fmt.Errorf("item %d has invalid id %q", i, item.ID)
		}
		if item.Name == "" || len(item.Name) > 160 {
			return nil, fmt.Errorf("item %q has an invalid name", item.ID)
		}
		if item.Icon != "" && (filepath.Base(item.Icon) != item.Icon || strings.ContainsAny(item.Icon, `/\\`)) {
			return nil, fmt.Errorf("item %q icon must be a file name", item.ID)
		}
		if item.MaxQuantity < 1 || item.MaxQuantity > 9999 {
			return nil, fmt.Errorf("item %q maxQuantity must be from 1 to 9999", item.ID)
		}
		key := strings.ToLower(item.ID)
		if _, exists := c.byID[key]; exists {
			return nil, fmt.Errorf("duplicate item id %q", item.ID)
		}
		c.byID[key] = item
		c.items = append(c.items, item)
		if item.Grantable {
			c.grantable++
		}
	}
	sort.Slice(c.items, func(i, j int) bool { return strings.ToLower(c.items[i].Name) < strings.ToLower(c.items[j].Name) })
	return c, nil
}

func (c *Catalog) GameVersion() string   { return c.gameVersion }
func (c *Catalog) Source() CatalogSource { return c.source }
func (c *Catalog) Count() int            { return len(c.items) }
func (c *Catalog) GrantableCount() int   { return c.grantable }

func (c *Catalog) Item(id string) (Item, bool) {
	item, ok := c.byID[strings.ToLower(strings.TrimSpace(id))]
	return item, ok
}

func (c *Catalog) Search(query string, limit, offset int) ([]Item, int) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := strings.ToLower(strings.TrimSpace(query))
	terms := strings.Fields(q)
	matches := make([]Item, 0)
	for _, item := range c.items {
		if !item.Grantable {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{item.Name, item.ID, item.Category, item.Rarity}, " "))
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, item)
		}
	}
	total := len(matches)
	if offset >= total {
		return []Item{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return append([]Item(nil), matches[offset:end]...), total
}
