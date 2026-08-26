package observability

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// contractMetrics is the exact set contracts/metrics.md says this feature
// registers. Metric names are a published contract: renaming one breaks
// dashboards and saved queries, so this test fails on additions as well as
// removals, forcing the contract file to be updated in the same change.
var contractMetrics = map[string][]string{
	"pulseflow_build_info":                        {"version", "commit", "go_version"},
	"pulseflow_http_requests_total":               {"route", "method", "code"},
	"pulseflow_http_request_duration_seconds":     {"route", "method"},
	"pulseflow_dependency_up":                     {"dependency"},
	"pulseflow_dependency_check_duration_seconds": {"dependency"},
}

// fullyRegistered builds a registry with everything this feature registers.
func fullyRegistered(t *testing.T) *prometheus.Registry {
	t.Helper()
	reg := NewRegistry(BuildInfo{Version: "v1", Commit: "abc123"})

	http := NewHTTPMetrics(reg)
	http.Observe("/v1/health/ready", "GET", 200, 5*time.Millisecond)

	deps := NewDependencyMetrics(reg)
	deps.RecordDependencyCheck("kafka", true, time.Millisecond)

	return reg
}

func TestRegisteredMetricsMatchTheContract(t *testing.T) {
	families, err := fullyRegistered(t).Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}

	got := map[string][]string{}
	for _, f := range families {
		name := f.GetName()
		if !strings.HasPrefix(name, Namespace+"_") {
			continue // go_* and process_* collectors keep their standard names
		}
		var labels []string
		if len(f.GetMetric()) > 0 {
			for _, pair := range f.GetMetric()[0].GetLabel() {
				labels = append(labels, pair.GetName())
			}
		}
		got[name] = labels
	}

	for name, wantLabels := range contractMetrics {
		gotLabels, ok := got[name]
		if !ok {
			t.Errorf("%s is in the contract but is not registered", name)
			continue
		}
		if !sameSet(gotLabels, wantLabels) {
			t.Errorf("%s labels = %v, want %v", name, gotLabels, wantLabels)
		}
	}

	for name := range got {
		if _, ok := contractMetrics[name]; !ok {
			t.Errorf("%s is registered but absent from contracts/metrics.md", name)
		}
	}
}

// Constitution Principle I: label values must come from a set the code
// controls. A value derived from request data makes cardinality unbounded.
func TestLabelValuesComeFromBoundedSets(t *testing.T) {
	families, err := fullyRegistered(t).Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}

	boundedValues := map[string]map[string]bool{
		"dependency": {"kafka": true, "clickhouse": true, "redis": true},
	}

	for _, f := range families {
		for _, m := range f.GetMetric() {
			for _, pair := range m.GetLabel() {
				allowed, checked := boundedValues[pair.GetName()]
				if !checked {
					continue
				}
				if !allowed[pair.GetValue()] {
					t.Errorf("%s has label %s=%q outside its bounded set",
						f.GetName(), pair.GetName(), pair.GetValue())
				}
			}
		}
	}
}

func TestBuildInfoIsAlwaysOne(t *testing.T) {
	families, err := fullyRegistered(t).Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}

	for _, f := range families {
		if f.GetName() != "pulseflow_build_info" {
			continue
		}
		metrics := f.GetMetric()
		if len(metrics) != 1 {
			t.Fatalf("build_info has %d series, want exactly 1", len(metrics))
		}
		if got := metrics[0].GetGauge().GetValue(); got != 1 {
			t.Errorf("build_info = %v, want 1", got)
		}
		return
	}
	t.Error("pulseflow_build_info was not registered")
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(got))
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}
