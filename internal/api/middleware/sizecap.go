package middleware

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
)

// SizeCap rejects requests whose Content-Length exceeds capForEndpoint(path)
// with 413 + standard app-error envelope. If capForEndpoint returns 0, no cap.
func SizeCap(capForEndpoint func(path string) int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			limit := capForEndpoint(c.Request().URL.Path)
			if limit > 0 {
				if cl := c.Request().Header.Get("Content-Length"); cl != "" {
					n, err := strconv.ParseInt(cl, 10, 64)
					if err == nil && n > limit {
						return apierror.WriteAppErrorEcho(c, http.StatusRequestEntityTooLarge, "Request body exceeds size cap", "")
					}
				}
			}
			return next(c)
		}
	}
}
