// Package web embeds the built Svelte SPA.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the SPA, falling back to index.html for client-side routes.
func Handler() http.Handler {
	sub, _ := fs.Sub(dist, "dist")
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			if _, ierr := fs.Stat(sub, "index.html"); ierr != nil {
				http.Error(w, "reet.sh UI is not built; run `npm run build` in web/", http.StatusNotFound)
				return
			}
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
