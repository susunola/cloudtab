package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/susunola/cloudtab/internal/policy"
)

func loadPolicyConfig(path string, maxTotals, maxIncreases []string, failOnSkipped, failOnSkippedSet bool, minCoverage float64, minCoverageSet bool) (policy.Config, bool, error) {
	enabled := path != "" || len(maxTotals) > 0 || len(maxIncreases) > 0 || failOnSkippedSet || minCoverageSet
	if !enabled {
		return policy.Config{}, false, nil
	}

	config := policy.Config{Version: policy.CurrentVersion, Limits: map[string]policy.CurrencyLimits{}}
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return policy.Config{}, false, fmt.Errorf("open policy file: %w", err)
		}
		defer file.Close()
		config, err = policy.Parse(file)
		if err != nil {
			return policy.Config{}, false, err
		}
		if config.Limits == nil {
			config.Limits = map[string]policy.CurrencyLimits{}
		}
	}

	if err := applyCurrencyThresholds(config.Limits, maxTotals, true); err != nil {
		return policy.Config{}, false, err
	}
	if err := applyCurrencyThresholds(config.Limits, maxIncreases, false); err != nil {
		return policy.Config{}, false, err
	}
	if failOnSkippedSet {
		value := failOnSkipped
		config.FailOnSkipped = &value
	}
	if minCoverageSet {
		value := minCoverage
		config.MinCoverage = &value
	}

	validated, err := policy.Validate(config)
	if err != nil {
		return policy.Config{}, false, err
	}
	return validated, true, nil
}

func applyCurrencyThresholds(limits map[string]policy.CurrencyLimits, values []string, total bool) error {
	seen := map[string]bool{}
	for _, raw := range values {
		currencyRaw, amountRaw, ok := strings.Cut(raw, "=")
		currency := strings.ToUpper(strings.TrimSpace(currencyRaw))
		if !ok || currency == "" || strings.TrimSpace(amountRaw) == "" {
			return fmt.Errorf("invalid policy threshold %q: want CURRENCY=AMOUNT", raw)
		}
		if seen[currency] {
			return fmt.Errorf("duplicate policy threshold for %s", currency)
		}
		seen[currency] = true
		amount, err := strconv.ParseFloat(strings.TrimSpace(amountRaw), 64)
		if err != nil {
			return fmt.Errorf("invalid policy threshold %q: %w", raw, err)
		}
		entry := limits[currency]
		if total {
			entry.MaxTotal = floatPtr(amount)
		} else {
			entry.MaxMonthlyIncrease = floatPtr(amount)
		}
		limits[currency] = entry
	}
	return nil
}

func floatPtr(value float64) *float64 { return &value }

func renderPolicyResult(w io.Writer, result policy.Result) {
	fmt.Fprintf(w, "\nCost policy: %s\n", strings.ToUpper(string(result.Status)))
	for _, violation := range result.Violations {
		fmt.Fprintf(w, "  - %s\n", violation.Message)
	}
}
