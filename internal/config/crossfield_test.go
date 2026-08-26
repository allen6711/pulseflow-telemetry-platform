package config

import (
	"strings"
	"testing"
	"time"
)

// A cached failure must not outlive the check that produced it. If the cache
// TTL exceeds the check timeout, a dependency that recovered stays reported as
// down for longer than FR-011's recovery window allows.
func TestHealthCacheTTLMustBeLessThanCheckTimeout(t *testing.T) {
	cases := []struct {
		name    string
		timeout string
		ttl     string
		wantErr bool
	}{
		{"ttl below timeout", "2s", "1s", false},
		{"ttl equal to timeout", "2s", "2s", true},
		{"ttl above timeout", "2s", "5s", true},
		{"zero ttl always re-checks", "2s", "0s", false},
		{"both raised, ordering kept", "10s", "5s", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvPrefix+"HEALTH_CHECK_TIMEOUT", tc.timeout)
			t.Setenv(EnvPrefix+"HEALTH_CACHE_TTL", tc.ttl)

			cfg, err := Load(APIDefaults)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ttl=%s timeout=%s was accepted", tc.ttl, tc.timeout)
				}
				if !strings.Contains(err.Error(), EnvPrefix+"HEALTH_CACHE_TTL") {
					t.Errorf("message does not name the offending setting:\n%v", err)
				}
				if !strings.Contains(err.Error(), EnvPrefix+"HEALTH_CHECK_TIMEOUT") {
					t.Errorf("message does not name the setting it conflicts with:\n%v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ttl=%s timeout=%s was rejected: %v", tc.ttl, tc.timeout, err)
			}
			wantTTL, _ := time.ParseDuration(tc.ttl)
			if cfg.HealthCacheTTL != wantTTL {
				t.Errorf("ttl = %s, want %s", cfg.HealthCacheTTL, wantTTL)
			}
		})
	}
}
