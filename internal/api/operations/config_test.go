package operations_test

import (
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
)

func TestLoad_NonProdDefaults(t *testing.T) {
	cfg := operations.Load(true)
	push := cfg.Endpoints[operations.EndpointPush]
	if push.RequestSizeCap != 4_000_000 {
		t.Errorf("non-prod push size cap = %d, want 4_000_000", push.RequestSizeCap)
	}
	if push.RateLimit.Requests != 60 {
		t.Errorf("non-prod push rate-limit requests = %d, want 60", push.RateLimit.Requests)
	}
}

func TestLoad_ProdDefaults(t *testing.T) {
	cfg := operations.Load(false)
	push := cfg.Endpoints[operations.EndpointPush]
	if push.RequestSizeCap != 2_000_000 {
		t.Errorf("prod push size cap = %d, want 2_000_000", push.RequestSizeCap)
	}
	if push.RateLimit.Requests != 30 {
		t.Errorf("prod push rate-limit requests = %d, want 30", push.RateLimit.Requests)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("API_PUSH_REQUEST_SIZE_CAP", "12345")
	t.Setenv("API_PUSH_RATE_LIMIT_REQUESTS", "7")

	cfg := operations.Load(true)
	push := cfg.Endpoints[operations.EndpointPush]
	if push.RequestSizeCap != 12345 {
		t.Errorf("override size cap = %d, want 12345", push.RequestSizeCap)
	}
	if push.RateLimit.Requests != 7 {
		t.Errorf("override rate-limit = %d, want 7", push.RateLimit.Requests)
	}
}

func TestEndpointKeyForPath(t *testing.T) {
	cases := map[string]string{
		"/api/v1/push":                    operations.EndpointPush,
		"/api/v1/feature-states":          operations.EndpointFeatureStates,
		"/api/v1/implementations":         operations.EndpointImplementations,
		"/api/v1/feature-context":         operations.EndpointFeatureContext,
		"/api/v1/implementation-features": operations.EndpointImplementationFeats,
		"/api/v1/random":                  operations.EndpointDefault,
	}
	for path, want := range cases {
		if got := operations.EndpointKeyForPath(path); got != want {
			t.Errorf("EndpointKeyForPath(%q) = %q, want %q", path, got, want)
		}
	}
}
