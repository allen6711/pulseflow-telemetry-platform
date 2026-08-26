package observability

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics counts and times HTTP requests.
//
// The route label carries the registered ServeMux pattern, never the concrete
// request path. That distinction is what keeps cardinality bounded once F06
// adds /v1/metrics/{service}/{metric}: labelling by path would mint a new time
// series for every service and metric ever queried.
type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTPMetrics registers the HTTP metrics against reg.
func NewHTTPMetrics(reg prometheus.Registerer) *HTTPMetrics {
	m := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "http_requests_total",
			Help:      "HTTP requests served, by route, method, and status code.",
		}, []string{"route", "method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency, by route and method.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"route", "method"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// Observe records one served request.
func (m *HTTPMetrics) Observe(route, method string, code int, d time.Duration) {
	m.requests.WithLabelValues(route, method, strconv.Itoa(code)).Inc()
	m.duration.WithLabelValues(route, method).Observe(d.Seconds())
}
