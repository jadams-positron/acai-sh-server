package google

// NewProviderForTest constructs a Provider with no OIDC discovery. Only
// fields exercised by Authorize / PostLoginRedirect are usable on the
// returned value; AuthCodeURL / Exchange will panic. Used by unit tests
// that don't need to round-trip through Google.
func NewProviderForTest(cfg Config) *Provider {
	return &Provider{cfg: cfg}
}
