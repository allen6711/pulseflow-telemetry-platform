package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DependencyMetrics observes readiness checks.
//
// The dependency label takes one of three fixed values, so cardinality is
// bounded by construction rather than by convention.
type DependencyMetrics struct {
	up       *prometheus.GaugeVec
	duration *prometheus.HistogramVec
}

// NewDependencyMetrics registers the dependency metrics against reg.
func NewDependencyMetrics(reg prometheus.Registerer) *DependencyMetrics {
	m := &DependencyMetrics{
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "dependency_up",
			Help:      "Result of the most recent readiness check per dependency: 1 healthy, 0 unhealthy.",
		}, []string{"dependency"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "dependency_check_duration_seconds",
			Help:      "Duration of readiness checks per dependency, including timeouts.",
			// Tuned for probes bounded at a couple of seconds rather than the
			// default request-latency spread.
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2, 5},
		}, []string{"dependency"}),
	}
	reg.MustRegister(m.up, m.duration)
	return m
}

// RecordDependencyCheck records one dependency check outcome.
func (m *DependencyMetrics) RecordDependencyCheck(name string, healthy bool, d time.Duration) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	m.up.WithLabelValues(name).Set(value)
	m.duration.WithLabelValues(name).Observe(d.Seconds())
}
