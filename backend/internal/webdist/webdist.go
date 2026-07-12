// Package webdist embeds the built frontend. The dist/ directory here is a
// committed placeholder; the release build replaces it with frontend/dist
// (see Makefile / Dockerfile) before compiling.
package webdist

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// FS is the embedded frontend rooted at the dist directory.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err) // impossible: dist is embedded at compile time
	}
	return sub
}
