package policy

import (
	"math"
	"strings"
	"testing"
)

func TestParseValidConfigNormalizesCurrencies(t *testing.T) {
	cfg, err := Parse(strings.NewReader(`
version: 1
limits:
  " usd ":
    max_monthly_increase: 250
    max_total: 5000
fail_on_skipped: true
min_coverage: 0.95
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("Version = %d, want 1", cfg.Version)
	}
	limits, ok := cfg.Limits["USD"]
	if !ok || len(cfg.Limits) != 1 {
		t.Fatalf("Limits = %#v, want one normalized USD entry", cfg.Limits)
	}
	if limits.MaxMonthlyIncrease == nil || *limits.MaxMonthlyIncrease != 250 {
		t.Fatalf("MaxMonthlyIncrease = %v, want 250", limits.MaxMonthlyIncrease)
	}
	if limits.MaxTotal == nil || *limits.MaxTotal != 5000 {
		t.Fatalf("MaxTotal = %v, want 5000", limits.MaxTotal)
	}
	if cfg.FailOnSkipped == nil || !*cfg.FailOnSkipped {
		t.Fatalf("FailOnSkipped = %v, want true", cfg.FailOnSkipped)
	}
	if cfg.MinCoverage == nil || *cfg.MinCoverage != 0.95 {
		t.Fatalf("MinCoverage = %v, want 0.95", cfg.MinCoverage)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	tests := map[string]string{
		"top level":      "version: 1\nfail_on_skipped: true\nextra: true\n",
		"currency limit": "version: 1\nlimits:\n  USD:\n    max_total: 1\n    extra: 2\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(input)); err == nil {
				t.Fatal("Parse accepted an unknown field")
			}
		})
	}
}

func TestParseRejectsInvalidVersionsAndDocuments(t *testing.T) {
	tests := map[string]string{
		"empty":                 "",
		"missing version":       "fail_on_skipped: true\n",
		"unsupported version":   "version: 2\nfail_on_skipped: true\n",
		"multiple documents":    "version: 1\nfail_on_skipped: true\n---\nversion: 1\nfail_on_skipped: true\n",
		"empty second document": "version: 1\nfail_on_skipped: true\n---\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(input)); err == nil {
				t.Fatal("Parse accepted invalid versioned YAML")
			}
		})
	}
}

func TestParseRejectsDuplicateNormalizedCurrencies(t *testing.T) {
	input := `
version: 1
limits:
  usd: {max_total: 10}
  USD: {max_total: 20}
`
	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse accepted duplicate normalized currencies")
	}
}

func TestParseRejectsInvalidRuleValues(t *testing.T) {
	tests := map[string]string{
		"blank currency":       "version: 1\nlimits:\n  ' ': {max_total: 1}\n",
		"empty currency rules": "version: 1\nlimits:\n  USD: {}\nfail_on_skipped: true\n",
		"negative increase":    "version: 1\nlimits:\n  USD: {max_monthly_increase: -1}\n",
		"negative total":       "version: 1\nlimits:\n  USD: {max_total: -1}\n",
		"nan increase":         "version: 1\nlimits:\n  USD: {max_monthly_increase: .nan}\n",
		"infinite total":       "version: 1\nlimits:\n  USD: {max_total: .inf}\n",
		"negative coverage":    "version: 1\nmin_coverage: -0.01\n",
		"coverage above one":   "version: 1\nmin_coverage: 1.01\n",
		"nan coverage":         "version: 1\nmin_coverage: .nan\n",
		"infinite coverage":    "version: 1\nmin_coverage: .inf\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(input)); err == nil {
				t.Fatal("Parse accepted an invalid rule value")
			}
		})
	}
}

func TestParseRejectsConfigsWithNoActiveRules(t *testing.T) {
	tests := map[string]string{
		"version only":          "version: 1\n",
		"empty limits":          "version: 1\nlimits: {}\n",
		"disabled skipped rule": "version: 1\nfail_on_skipped: false\n",
		"zero coverage":         "version: 1\nmin_coverage: 0\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(input)); err == nil {
				t.Fatal("Parse accepted a config with no active rules")
			}
		})
	}
}

func TestValidateSupportsProgrammaticConfigMergingWithoutMutation(t *testing.T) {
	maxTotal := 0.0
	cfg := Config{
		Version: 1,
		Limits: map[string]CurrencyLimits{
			"usd": {MaxTotal: &maxTotal},
		},
	}
	normalized, err := Validate(cfg)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, ok := normalized.Limits["USD"]; !ok {
		t.Fatalf("normalized Limits = %#v, want USD", normalized.Limits)
	}
	if _, ok := cfg.Limits["usd"]; !ok {
		t.Fatalf("Validate mutated input Limits: %#v", cfg.Limits)
	}
}

func TestValidateRejectsNonFiniteProgrammaticValues(t *testing.T) {
	nan := math.NaN()
	cfg := Config{Version: 1, MinCoverage: &nan}
	if _, err := Validate(cfg); err == nil {
		t.Fatal("Validate accepted NaN coverage")
	}
}
