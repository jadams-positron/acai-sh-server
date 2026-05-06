package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
)

func TestMountStatic_ServesDatastarJS(t *testing.T) {
	e := echo.New()
	handlers.MountStatic(e.Group(""))
	ts := httptest.NewServer(e)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/_assets/js/datastar.min.js", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /_assets/js/datastar.min.js: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if cc := resp.Header.Get("Cache-Control"); cc == "" {
		t.Error("Cache-Control header missing")
	}
}

func TestMountStatic_404OnMissingAsset(t *testing.T) {
	e := echo.New()
	handlers.MountStatic(e.Group(""))
	ts := httptest.NewServer(e)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/_assets/js/nonexistent.js", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /_assets/js/nonexistent.js: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
