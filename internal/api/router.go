// Package api mounts the /api/v1/* sub-router with huma, bearer auth,
// size-cap, and rate-limit middleware. Actual operation registration happens
// in P2b/P2c.
package api

import (
	// huma is the OpenAPI 3.1 framework; used here to anchor the dep until the
	// full router wiring lands in T9.
	_ "github.com/danielgtaylor/huma/v2"
)
