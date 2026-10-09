package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spaHandler serves the built Vue bundle and falls back to index.html so the
// client router owns deep links.
//
// In production nginx serves these files directly and only /api/ reaches Go.
// This exists so the binary can also be run on its own — one process, one
// port — which is how the app is tested before nginx is pointed at it.
func spaHandler(root string) http.HandlerFunc {
	fs := http.Dir(root)
	fileServer := http.FileServer(fs)
	return func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)
		if clean == "/" {
			clean = "/index.html"
		}
		full := filepath.Join(root, filepath.FromSlash(clean))
		// Refuse anything that escaped the root.
		if !strings.HasPrefix(full, filepath.Clean(root)+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			// Vite names built assets with a content hash, so they can be
			// cached forever; index.html must not be.
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(clean, "/api/") {
			writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
			return
		}
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
}
