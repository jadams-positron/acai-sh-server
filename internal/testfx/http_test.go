package testfx_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// newPingEcho returns a minimal echo instance with a single GET /ping route.
func newPingEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"pong": "true"})
	})
	e.GET("/echo-auth", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"auth": c.Request().Header.Get("Authorization"),
		})
	})
	e.GET("/query", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"foo": c.QueryParam("foo"),
		})
	})
	return e
}

func TestHTTPClient_GET_200(t *testing.T) {
	e := newPingEcho()
	client := testfx.HTTPClient(t, e)
	resp := client.GET("/ping", nil)
	resp.AssertStatus(http.StatusOK)
}

func TestHTTPClient_WithBearer_SetsHeader(t *testing.T) {
	e := newPingEcho()
	client := testfx.HTTPClient(t, e).WithBearer("tok123")
	resp := client.GET("/echo-auth", nil)
	resp.AssertStatus(http.StatusOK)

	var body map[string]string
	resp.JSON(&body)
	if body["auth"] != "Bearer tok123" {
		t.Errorf("auth = %q, want %q", body["auth"], "Bearer tok123")
	}
}

func TestHTTPClient_GET_WithQuery(t *testing.T) {
	e := newPingEcho()
	client := testfx.HTTPClient(t, e)
	q := url.Values{"foo": {"bar"}}
	resp := client.GET("/query", q)
	resp.AssertStatus(http.StatusOK)

	var body map[string]string
	resp.JSON(&body)
	if body["foo"] != "bar" {
		t.Errorf("foo = %q, want bar", body["foo"])
	}
}

func TestResponse_AssertJSONContains(t *testing.T) {
	e := newPingEcho()
	client := testfx.HTTPClient(t, e)
	resp := client.GET("/ping", nil)
	resp.AssertStatus(http.StatusOK).AssertJSONContains(map[string]any{"pong": "true"})
}

func TestHTTPClient_WithBearer_DoesNotMutateOriginal(t *testing.T) {
	e := newPingEcho()
	base := testfx.HTTPClient(t, e)
	_ = base.WithBearer("tok-a")
	// base should not have bearer set
	resp := base.GET("/echo-auth", nil)
	var body map[string]string
	resp.JSON(&body)
	if body["auth"] != "" {
		t.Errorf("original client auth = %q, want empty (WithBearer should not mutate)", body["auth"])
	}
}
