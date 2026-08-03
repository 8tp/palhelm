// Command pal-passive-catalog-import turns PalCalc's pinned db.json into the
// compact, reviewed passive catalogue embedded by the Pal grant API.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

const sourceCommit = "b5e13e90fedc2e95d54fa223da77be464c313001"

type sourceDB struct {
	Version       string          `json:"Version"`
	PassiveSkills []sourcePassive `json:"PassiveSkills"`
}

type sourcePassive struct {
	Name                     string `json:"Name"`
	InternalName             string `json:"InternalName"`
	Rank                     int    `json:"Rank"`
	Description              string `json:"Description"`
	IsStandardPassiveSkill   bool   `json:"IsStandardPassiveSkill"`
	RandomInheritanceAllowed bool   `json:"RandomInheritanceAllowed"`
}

type outputCatalog struct {
	SchemaVersion  int             `json:"schemaVersion"`
	CatalogVersion string          `json:"catalogVersion"`
	Source         outputSource    `json:"source"`
	Passives       []outputPassive `json:"passives"`
}

type outputSource struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	URL     string `json:"url"`
}

type outputPassive struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Rank        int    `json:"rank"`
	Inheritable bool   `json:"inheritable"`
}

func main() {
	input := flag.String("in", "", "PalCalc db.json path")
	output := flag.String("out", "", "generated catalogue path")
	flag.Parse()
	if *input == "" || *output == "" {
		fatalf("both -in and -out are required")
	}
	b, err := os.ReadFile(*input)
	if err != nil {
		fatalf("read input: %v", err)
	}
	var db sourceDB
	if err := json.Unmarshal(b, &db); err != nil {
		fatalf("decode input: %v", err)
	}
	if strings.TrimSpace(db.Version) == "" || len(db.PassiveSkills) < 100 {
		fatalf("source database does not look complete")
	}
	seen := make(map[string]bool)
	out := outputCatalog{
		SchemaVersion:  1,
		CatalogVersion: "palcalc_" + strings.ToLower(strings.TrimSpace(db.Version)) + "_" + sourceCommit[:12],
		Source: outputSource{
			Name:    "PalCalc",
			Version: strings.TrimSpace(db.Version),
			Commit:  sourceCommit,
			URL:     "https://github.com/tylercamp/palcalc/tree/" + sourceCommit,
		},
	}
	for _, item := range db.PassiveSkills {
		id, name := strings.TrimSpace(item.InternalName), strings.TrimSpace(item.Name)
		key := strings.ToLower(id)
		if !item.IsStandardPassiveSkill || id == "" || name == "" || seen[key] {
			continue
		}
		seen[key] = true
		out.Passives = append(out.Passives, outputPassive{
			ID:          id,
			Name:        name,
			Description: strings.TrimSpace(item.Description),
			Rank:        item.Rank,
			Inheritable: item.RandomInheritanceAllowed,
		})
	}
	if len(out.Passives) < 100 {
		fatalf("only %d standard passives found", len(out.Passives))
	}
	sort.Slice(out.Passives, func(i, j int) bool {
		if out.Passives[i].Rank != out.Passives[j].Rank {
			return out.Passives[i].Rank > out.Passives[j].Rank
		}
		return strings.ToLower(out.Passives[i].Name) < strings.ToLower(out.Passives[j].Name)
	})
	b, err = json.MarshalIndent(out, "", "  ")
	if err != nil {
		fatalf("encode output: %v", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(*output, b, 0o644); err != nil {
		fatalf("write output: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
