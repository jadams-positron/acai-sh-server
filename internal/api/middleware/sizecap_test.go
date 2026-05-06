package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
)

func TestSizeCap_AllowsUnderCap(t *testing.T) {
	e := echo.New()
	e.Use(middleware.SizeCap(func(string) int64 { return 1024 }))
	e.POST("/x", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hello"))
	req.ContentLength = 5
	req.Header.Set("Content-Length", "5")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSizeCap_RejectsOverCap(t *testing.T) {
	e := echo.New()
	e.Use(middleware.SizeCap(func(string) int64 { return 100 }))
	e.POST("/x", func(c echo.Context) error {
		t.Errorf("downstream should not be called")
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Content-Length", "1024")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestSizeCap_NoCap(t *testing.T) {
	e := echo.New()
	e.Use(middleware.SizeCap(func(string) int64 { return 0 }))
	e.POST("/x", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Content-Length", "9999999")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no cap)", rec.Code)
	}
}
