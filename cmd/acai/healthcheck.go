package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// runHealthcheck performs an HTTP GET to localhost:$HTTP_PORT/_health and
// exits 0 when the response is HTTP 200. It is used as the Docker healthcheck
// probe so that `CMD ["/acai", "healthcheck"]` works without additional tooling.
func runHealthcheck(_ context.Context, stderr io.Writer) int {
	port := 4000
	if p := os.Getenv("HTTP_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			port = n
		}
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/_health", port)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url) //nolint:noctx,gosec // healthcheck probes a fixed localhost URL; no SSRF risk
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "healthcheck: unexpected status %d from %s\n", resp.StatusCode, url)
	return 1
}
