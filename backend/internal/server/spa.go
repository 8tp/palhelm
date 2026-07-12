package server

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/8tp/palhelm/internal/webdist"
)

// spaHandler serves the embedded frontend: real files with sensible caching,
// and index.html for every non-API path so client-side routes deep-link.
func spaHandler() http.Handler {
	dist := webdist.FS()
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := dist.Open(p); err == nil {
			_ = f.Close()
			// Vite emits content-hashed filenames under assets/ — cache those hard.
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: unknown paths get the app shell (client router handles 404s).
		serveIndex(w, dist)
	})
}

func serveIndex(w http.ResponseWriter, dist fs.FS) {
	f, err := dist.Open("index.html")
	if err != nil {
		http.Error(w, "frontend not embedded", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, f)
}

// PanelVersion is stamped by cmd/palhelm from the build-time version.
var PanelVersion = "dev"
