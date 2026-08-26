// Package observability owns PulseFlow's metric registry.
//
// The registry is explicit rather than prometheus.DefaultRegisterer: tests can
// build an isolated one and assert on metric names without global state leaking
// between cases, and later features register into the same instance instead of
// each creating their own. Naming and label cardinality rules are fixed by
// contracts/metrics.md.
package observability

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Namespace prefixes every metric this project defines.
const Namespace = "pulseflow"

// BuildInfo identifies the running binary.
type BuildInfo struct {
	Version string
	Commit  string
}

// NewRegistry returns a registry carrying build information plus the standard
// Go runtime and process collectors.
func NewRegistry(info BuildInfo) *prometheus.Registry {
	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "build_info",
		Help:      "Build identification for the running binary. Always 1.",
	}, []string{"version", "commit", "go_version"})
	buildInfo.WithLabelValues(info.Version, info.Commit, runtime.Version()).Set(1)
	reg.MustRegister(buildInfo)

	return reg
}
