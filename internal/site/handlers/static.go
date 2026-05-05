package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jadams-positron/acai-sh-server/assets"
)

// MountStatic registers the /_assets/* file-server route on r.
// All assets are served with a 5-minute public cache header.
// This must be mounted BEFORE session-loaded route groups so
// browsers can fetch assets without a session cookie.
func MountStatic(r chi.Router) {
	fs := http.FileServerFS(assets.FS)
	r.Get("/_assets/*", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		// Strip the /_assets prefix so the FileServer sees the bare path
		// (e.g. "js/datastar.min.js") rooted at the embedded FS.
		http.StripPrefix("/_assets/", fs).ServeHTTP(w, r)
	})
}
