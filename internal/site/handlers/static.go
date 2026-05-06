package handlers

import (
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/assets"
)

// MountStatic registers the /_assets/* file-server route on g.
// All assets are served with a 5-minute public cache header.
func MountStatic(g *echo.Group) {
	sub, err := fs.Sub(assets.FS, ".")
	if err != nil {
		panic("assets: fs.Sub: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	g.GET("/_assets/*", echo.WrapHandler(http.StripPrefix("/_assets/", cacheHeaders(fileServer))))
}

func cacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		next.ServeHTTP(w, r)
	})
}
