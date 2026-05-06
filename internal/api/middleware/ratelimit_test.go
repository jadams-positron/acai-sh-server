package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
)

func TestRateLimit_AllowsBelowQuota(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 3, WindowSeconds: 60}

	for i := range 3 {
		if !limiter.Allow("ep", "tok", spec, time.Now()) {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}
}

func TestRateLimit_RejectsOverQuota(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 2, WindowSeconds: 60}

	now := time.Now()
	_ = limiter.Allow("ep", "tok", spec, now)
	_ = limiter.Allow("ep", "tok", spec, now)
	if limiter.Allow("ep", "tok", spec, now) {
		t.Errorf("third call should be rejected")
	}
}

func TestRateLimit_DifferentTokensIndependent(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 1, WindowSeconds: 60}

	now := time.Now()
	if !limiter.Allow("ep", "tokA", spec, now) {
		t.Errorf("first call for tokA should be allowed")
	}
	if !limiter.Allow("ep", "tokB", spec, now) {
		t.Errorf("first call for tokB should be allowed (independent)")
	}
}

func TestRateLimit_NewWindowResets(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 1, WindowSeconds: 1}

	t0 := time.Unix(1000, 0)
	t1 := time.Unix(1001, 0)

	if !limiter.Allow("ep", "tok", spec, t0) {
		t.Errorf("first call should be allowed")
	}
	if limiter.Allow("ep", "tok", spec, t0) {
		t.Errorf("second call in same bucket should be rejected")
	}
	if !limiter.Allow("ep", "tok", spec, t1) {
		t.Errorf("first call in new bucket should be allowed")
	}
}

func TestRateLimit_Middleware_429OnExceeded(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 1, WindowSeconds: 60}

	e := echo.New()
	e.Use(middleware.RateLimit(func(string) middleware.RateLimitSpec { return spec }, limiter))
	e.GET("/x", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	doRequest := func() int {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "/x", http.NoBody)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := doRequest(); got != http.StatusOK {
		t.Errorf("first request code = %d, want 200", got)
	}
	if got := doRequest(); got != http.StatusTooManyRequests {
		t.Errorf("second request code = %d, want 429", got)
	}
}
