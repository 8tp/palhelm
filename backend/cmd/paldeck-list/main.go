// Command paldeck-list prints Palhelm's CharacterID→display-name table as tab-separated
// "id\tname" lines, one per Pal, sorted by id.
//
// It is the single source of truth scripts/fetch-pal-icons.sh reads to know which Pals to fetch
// icons for: rather than maintaining a second, hand-copied list of internal names that could
// drift out of sync with internal/paldeck, the fetch script just runs
//
//	go run ./cmd/paldeck-list
//
// from the backend module directory and iterates the output. See internal/paldeck.All for the
// merge/sort rules.
package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/palhelm/palhelm/internal/paldeck"
)

func main() {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for _, e := range paldeck.All() {
		fmt.Fprintf(w, "%s\t%s\n", e.ID, e.Name)
	}
}
