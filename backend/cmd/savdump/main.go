// Command savdump prints a parsed Palworld save as JSON.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/palhelm/palhelm/internal/sav"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <file.sav>\n", filepath.Base(os.Args[0]))
		os.Exit(2)
	}
	var value any
	var err error
	if strings.EqualFold(filepath.Base(os.Args[1]), "LevelMeta.sav") {
		value, err = sav.ParseLevelMeta(os.Args[1])
	} else {
		value, err = sav.ParseLevel(os.Args[1], sav.Options{})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "savdump: %v\n", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		fmt.Fprintf(os.Stderr, "savdump: encode JSON: %v\n", err)
		os.Exit(1)
	}
}
