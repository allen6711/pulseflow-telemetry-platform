// Package health implements PulseFlow's two health signals.
//
// They answer deliberately different questions. Liveness answers "is this
// process alive?" and drives restart decisions, so it never consults an
// external dependency -- otherwise a brief dependency outage would restart
// every replica. Readiness answers "can this process serve correctly right
// now?" and drives traffic routing, so it does. The response shapes are fixed
// by contracts/health-api.yaml.
package health

import (
	"encoding/json"
	"net/http"
)

// Route patterns, also used as bounded metric label values.
const (
	LivePath  = "/v1/health/live"
	ReadyPath = "/v1/health/ready"
)

// LivenessResponse is the body of a liveness probe response.
type LivenessResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

// StatusAlive is the only status a liveness probe reports.
const StatusAlive = "alive"

// LivenessHandler always reports success while the process can serve HTTP. It
// takes no dependencies by construction, not by convention, so a later change
// cannot quietly couple it to one.
func LivenessHandler(service, version string) http.Handler {
	body, err := json.Marshal(LivenessResponse{
		Status:  StatusAlive,
		Service: service,
		Version: version,
	})
	if err != nil {
		// The struct is three strings; marshalling it cannot fail.
		panic("health: marshalling liveness response: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

// ReadinessHandler reports whether this process can serve correctly.
//
// It returns 503 when any dependency is unhealthy or the process is shutting
// down, and in both cases the body still lists every dependency with its own
// status: a probe should show the whole picture, not just the first failure
// encountered (FR-008).
func ReadinessHandler(agg *Aggregator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := agg.Evaluate(r.Context())

		code := http.StatusOK
		if result.Status != StatusReady {
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)

		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(result); err != nil {
			// The response is already committed; nothing useful remains.
			return
		}
	})
}
