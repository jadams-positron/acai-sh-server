package ops

import (
	"encoding/json"
	"net/http"

	"github.com/acai-sh/server/internal/store"
)

// healthResponse is the JSON payload returned by /_health.
type healthResponse struct {
	Status  string `json:"status"`
	DB      string `json:"db"`
	Version string `json:"version"`
}

// HealthHandler returns an http.Handler that checks DB reachability and reports
// service health. It writes:
//   - 200 {"status":"ok","db":"ok","version":"<v>"} when the DB is reachable.
//   - 503 {"status":"degraded","db":"error","version":"<v>"} when it is not.
func HealthHandler(db *store.DB, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := healthResponse{
			Status:  "ok",
			DB:      "ok",
			Version: version,
		}
		code := http.StatusOK

		if err := db.Read.PingContext(r.Context()); err != nil {
			resp.Status = "degraded"
			resp.DB = "error"
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	})
}
