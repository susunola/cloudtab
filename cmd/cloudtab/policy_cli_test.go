package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPolicyConfigMergesFileAndCLIOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yml")
	body := `version: 1
limits:
  USD:
    max_total: 100
    max_monthly_increase: 20
fail_on_skipped: false
min_coverage: 0.8
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	config, enabled, err := loadPolicyConfig(path, []string{"USD=90", "CNY=500"}, []string{"USD=10"}, true, true, 0.95, true)
	if err != nil {
		t.Fatalf("loadPolicyConfig: %v", err)
	}
	if !enabled {
		t.Fatal("policy was not enabled")
	}
	if got := *config.Limits["USD"].MaxTotal; got != 90 {
		t.Fatalf("USD max_total = %v, want CLI override 90", got)
	}
	if got := *config.Limits["USD"].MaxMonthlyIncrease; got != 10 {
		t.Fatalf("USD max increase = %v, want CLI override 10", got)
	}
	if got := *config.Limits["CNY"].MaxTotal; got != 500 {
		t.Fatalf("CNY max_total = %v, want 500", got)
	}
	if config.FailOnSkipped == nil || !*config.FailOnSkipped {
		t.Fatal("fail_on_skipped CLI override was not applied")
	}
	if config.MinCoverage == nil || math.Abs(*config.MinCoverage-0.95) > 1e-12 {
		t.Fatalf("min_coverage = %v, want 0.95", config.MinCoverage)
	}
}

func TestLoadPolicyConfigRejectsInvalidAndDuplicateThresholds(t *testing.T) {
	tests := [][]string{
		{"USD"},
		{"=1"},
		{"USD=bad"},
		{"USD=-1"},
		{"USD=1", "usd=2"},
	}
	for _, values := range tests {
		if _, _, err := loadPolicyConfig("", values, nil, false, false, 0, false); err == nil {
			t.Errorf("thresholds %v returned nil error", values)
		}
	}
}

func TestLoadPolicyConfigDisabledByDefault(t *testing.T) {
	_, enabled, err := loadPolicyConfig("", nil, nil, false, false, 0, false)
	if err != nil || enabled {
		t.Fatalf("default policy = enabled %v, error %v; want disabled", enabled, err)
	}
}
