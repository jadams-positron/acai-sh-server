// Package operations holds the runtime per-endpoint API configuration:
// request-size caps, semantic caps, and rate-limit specs. Mirrors the env-var
// shape from the original Phoenix runtime.exs.
package operations

import (
	"os"
	"strconv"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
)

// EndpointConfig is the per-endpoint runtime config.
type EndpointConfig struct {
	RequestSizeCap int64
	RateLimit      middleware.RateLimitSpec
	SemanticCaps   map[string]int
}

// Config holds per-endpoint config keyed by endpoint name.
type Config struct {
	NonProd   bool
	Endpoints map[string]EndpointConfig
}

// Endpoint keys (matching the Phoenix Operations.endpoint_key mapping).
const (
	EndpointDefault             = "default"
	EndpointPush                = "push"
	EndpointFeatureStates       = "feature_states"
	EndpointImplementations     = "implementations"
	EndpointFeatureContext      = "feature_context"
	EndpointImplementationFeats = "implementation_features"
)

// Load reads operation config from environment variables. Defaults match
// the non-prod profile from runtime.exs.
func Load(nonProd bool) *Config {
	cfg := &Config{NonProd: nonProd, Endpoints: map[string]EndpointConfig{}}

	pickInt := func(key string, prod, devDefault int) int {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			n, err := strconv.Atoi(v)
			if err == nil {
				return n
			}
		}
		if nonProd {
			return devDefault
		}
		return prod
	}
	pickInt64 := func(key string, prod, devDefault int64) int64 {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				return n
			}
		}
		if nonProd {
			return devDefault
		}
		return prod
	}

	cfg.Endpoints[EndpointDefault] = EndpointConfig{
		RequestSizeCap: pickInt64("API_DEFAULT_REQUEST_SIZE_CAP", 1_000_000, 2_000_000),
		RateLimit: middleware.RateLimitSpec{
			Requests:      pickInt("API_DEFAULT_RATE_LIMIT_REQUESTS", 60, 120),
			WindowSeconds: pickInt("API_DEFAULT_RATE_LIMIT_WINDOW_SECONDS", 60, 60),
		},
		SemanticCaps: map[string]int{
			"max_specs":      pickInt("API_DEFAULT_MAX_SPECS", 50, 100),
			"max_references": pickInt("API_DEFAULT_MAX_REFERENCES", 5_000, 10_000),
		},
	}

	cfg.Endpoints[EndpointPush] = EndpointConfig{
		RequestSizeCap: pickInt64("API_PUSH_REQUEST_SIZE_CAP", 2_000_000, 4_000_000),
		RateLimit: middleware.RateLimitSpec{
			Requests:      pickInt("API_PUSH_RATE_LIMIT_REQUESTS", 30, 60),
			WindowSeconds: pickInt("API_PUSH_RATE_LIMIT_WINDOW_SECONDS", 60, 60),
		},
		SemanticCaps: map[string]int{
			"max_specs":                      pickInt("API_PUSH_MAX_SPECS", 50, 100),
			"max_references":                 pickInt("API_PUSH_MAX_REFERENCES", 5_000, 10_000),
			"max_requirements_per_spec":      pickInt("API_PUSH_MAX_REQUIREMENTS_PER_SPEC", 100, 200),
			"max_raw_content_bytes":          pickInt("API_PUSH_MAX_RAW_CONTENT_BYTES", 51_200, 102_400),
			"max_requirement_string_length":  pickInt("API_PUSH_MAX_REQUIREMENT_STRING_LENGTH", 1_000, 2_000),
			"max_feature_description_length": pickInt("API_PUSH_MAX_FEATURE_DESCRIPTION_LENGTH", 2_500, 5_000),
			"max_meta_path_length":           pickInt("API_PUSH_MAX_META_PATH_LENGTH", 512, 1_024),
			"max_repo_uri_length":            pickInt("API_PUSH_MAX_REPO_URI_LENGTH", 1_024, 2_048),
		},
	}

	cfg.Endpoints[EndpointFeatureStates] = EndpointConfig{
		RequestSizeCap: pickInt64("API_FEATURE_STATES_REQUEST_SIZE_CAP", 1_000_000, 2_000_000),
		RateLimit: middleware.RateLimitSpec{
			Requests:      pickInt("API_FEATURE_STATES_RATE_LIMIT_REQUESTS", 30, 60),
			WindowSeconds: pickInt("API_FEATURE_STATES_RATE_LIMIT_WINDOW_SECONDS", 60, 60),
		},
		SemanticCaps: map[string]int{
			"max_states":         pickInt("API_FEATURE_STATES_MAX_STATES", 500, 500),
			"max_comment_length": pickInt("API_FEATURE_STATES_MAX_COMMENT_LENGTH", 2_000, 2_000),
		},
	}

	return cfg
}

// EndpointKeyForPath returns the endpoint key for a request path.
func EndpointKeyForPath(path string) string {
	switch path {
	case "/api/v1/push":
		return EndpointPush
	case "/api/v1/feature-states":
		return EndpointFeatureStates
	case "/api/v1/implementations":
		return EndpointImplementations
	case "/api/v1/feature-context":
		return EndpointFeatureContext
	case "/api/v1/implementation-features":
		return EndpointImplementationFeats
	default:
		return EndpointDefault
	}
}

// SizeCapForPath returns the per-endpoint request size cap (or default).
func (c *Config) SizeCapForPath(path string) int64 {
	key := EndpointKeyForPath(path)
	if ep, ok := c.Endpoints[key]; ok {
		return ep.RequestSizeCap
	}
	return c.Endpoints[EndpointDefault].RequestSizeCap
}

// RateLimitForPath returns the per-endpoint rate-limit spec (or default).
func (c *Config) RateLimitForPath(path string) middleware.RateLimitSpec {
	key := EndpointKeyForPath(path)
	if ep, ok := c.Endpoints[key]; ok {
		return ep.RateLimit
	}
	return c.Endpoints[EndpointDefault].RateLimit
}
