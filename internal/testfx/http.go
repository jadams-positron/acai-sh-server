package testfx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// Client wraps an *echo.Echo for tests. It does NOT bind a real listener —
// requests are dispatched in-process via e.ServeHTTP. Each Request constructs
// a fresh *httptest.ResponseRecorder.
type Client struct {
	t       *testing.T
	echo    *echo.Echo
	headers map[string]string
}

// HTTPClient returns a *Client over e. Use Client methods to issue requests.
func HTTPClient(t *testing.T, e *echo.Echo) *Client {
	t.Helper()
	return &Client{t: t, echo: e, headers: map[string]string{}}
}

// WithBearer returns a copy of c with Authorization: Bearer <plaintext>.
func (c *Client) WithBearer(plaintext string) *Client {
	cc := c.clone()
	cc.headers["Authorization"] = "Bearer " + plaintext
	return cc
}

// WithHeader returns a copy of c with the given header set.
func (c *Client) WithHeader(key, value string) *Client {
	cc := c.clone()
	cc.headers[key] = value
	return cc
}

func (c *Client) clone() *Client {
	cp := &Client{t: c.t, echo: c.echo, headers: make(map[string]string, len(c.headers))}
	maps.Copy(cp.headers, c.headers)
	return cp
}

// GET issues a GET to path (with optional query params encoded in path).
func (c *Client) GET(path string, query url.Values) *Response {
	return c.do(http.MethodGet, path, query, nil, "")
}

// POSTJSON issues a POST with a JSON body marshaled from body (must be JSON-marshallable).
func (c *Client) POSTJSON(path string, body any) *Response {
	c.t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			c.t.Fatalf("testfx.Client.POSTJSON: marshal: %v", err)
		}
	}
	return c.do(http.MethodPost, path, nil, buf, "application/json")
}

// PATCHJSON issues a PATCH with JSON body.
func (c *Client) PATCHJSON(path string, body any) *Response {
	c.t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			c.t.Fatalf("testfx.Client.PATCHJSON: marshal: %v", err)
		}
	}
	return c.do(http.MethodPatch, path, nil, buf, "application/json")
}

// PATCHRaw issues a PATCH with a raw body and explicit content type.
func (c *Client) PATCHRaw(path, contentType string, body io.Reader) *Response {
	return c.do(http.MethodPatch, path, nil, body, contentType)
}

// POSTRaw issues a POST with a raw body and explicit content type.
func (c *Client) POSTRaw(path, contentType string, body io.Reader) *Response {
	return c.do(http.MethodPost, path, nil, body, contentType)
}

// POSTForm issues a POST with form-encoded body.
func (c *Client) POSTForm(path string, form url.Values) *Response {
	body := strings.NewReader(form.Encode())
	return c.do(http.MethodPost, path, nil, body, "application/x-www-form-urlencoded")
}

func (c *Client) do(method, path string, query url.Values, body io.Reader, contentType string) *Response {
	c.t.Helper()
	if len(query) > 0 {
		path = path + "?" + query.Encode()
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, body)
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	c.echo.ServeHTTP(rec, req)
	return &Response{t: c.t, rec: rec}
}

// Response wraps the recorded HTTP response with assertion helpers.
type Response struct {
	t   *testing.T
	rec *httptest.ResponseRecorder
}

// Status returns the HTTP status code.
func (r *Response) Status() int { return r.rec.Code }

// Body returns the raw response body bytes.
func (r *Response) Body() []byte { return r.rec.Body.Bytes() }

// Header returns the named response header value.
func (r *Response) Header(key string) string { return r.rec.Header().Get(key) }

// Cookies returns the Set-Cookie cookies from the response.
func (r *Response) Cookies() []*http.Cookie {
	res := http.Response{Header: r.rec.Header()}
	return res.Cookies()
}

// JSON decodes the body into out. Fails the test on decode error.
func (r *Response) JSON(out any) *Response {
	r.t.Helper()
	if err := json.Unmarshal(r.rec.Body.Bytes(), out); err != nil {
		r.t.Fatalf("testfx.Response.JSON: decode: %v\nbody=%s", err, r.rec.Body.String())
	}
	return r
}

// AssertStatus fails the test if the status code doesn't match.
func (r *Response) AssertStatus(want int) *Response {
	r.t.Helper()
	if r.rec.Code != want {
		r.t.Fatalf("testfx: status = %d, want %d; body=%s", r.rec.Code, want, r.rec.Body.String())
	}
	return r
}

// AssertJSONContains parses the body as JSON and verifies that all keys in
// 'want' have matching values via deep equality. Useful for shape-matching
// without locking down the entire response.
func (r *Response) AssertJSONContains(want map[string]any) *Response {
	r.t.Helper()
	var got map[string]any
	if err := json.Unmarshal(r.rec.Body.Bytes(), &got); err != nil {
		r.t.Fatalf("testfx: body is not JSON: %v\nbody=%s", err, r.rec.Body.String())
	}
	for k, w := range want {
		if g, ok := got[k]; !ok || !deepEqualJSON(g, w) {
			r.t.Fatalf("testfx: key %q = %v, want %v\nbody=%s", k, g, w, r.rec.Body.String())
		}
	}
	return r
}

func deepEqualJSON(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}
