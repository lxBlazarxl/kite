// Package web embeds the built dashboard and serves it.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the dashboard built into dist/ by `npm run build` in web/.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return newHandler(sub)
}

func newHandler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		st, err := fs.Stat(files, name)
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			// Vite puts a content hash in every asset name.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFileFS(w, r, files, name)
	})
}
