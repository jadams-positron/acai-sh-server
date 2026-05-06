package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
)

func TestSizeCap_AllowsUnderCap(t *testing.T) {
	mw := middleware.SizeCap(func(string) int64 { return 1024 })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", strings.NewReader("hello"))
	req.ContentLength = 5
	req.Header.Set("Content-Length", "5")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSizeCap_RejectsOverCap(t *testing.T) {
	mw := middleware.SizeCap(func(string) int64 { return 100 })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("downstream should not be called")
	}))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Content-Length", "1024")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestSizeCap_NoCap(t *testing.T) {
	mw := middleware.SizeCap(func(string) int64 { return 0 })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Content-Length", "9999999")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no cap)", rec.Code)
	}
}
