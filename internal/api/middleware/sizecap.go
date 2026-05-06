package middleware

import (
	"net/http"
	"strconv"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
)

// SizeCap rejects requests whose Content-Length exceeds capForEndpoint(path)
// with 413 + standard app-error envelope. If capForEndpoint returns 0, no cap.
func SizeCap(capForEndpoint func(path string) int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := capForEndpoint(r.URL.Path)
			if limit > 0 {
				if cl := r.Header.Get("Content-Length"); cl != "" {
					n, err := strconv.ParseInt(cl, 10, 64)
					if err == nil && n > limit {
						apierror.WriteAppError(w, http.StatusRequestEntityTooLarge,
							"Request body exceeds size cap", "")
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
