// Package policy parses and evaluates cost policies without performing I/O.
package policy

import (
	"fmt"
	"io"
	"math"
	"strings"

	"gopkg.in/yaml.v3"
)

const CurrentVersion = 1

type Config struct {
	Version       int                       `yaml:"version" json:"version"`
	Limits        map[string]CurrencyLimits `yaml:"limits,omitempty" json:"limits,omitempty"`
	FailOnSkipped *bool                     `yaml:"fail_on_skipped,omitempty" json:"fail_on_skipped,omitempty"`
	MinCoverage   *float64                  `yaml:"min_coverage,omitempty" json:"min_coverage,omitempty"`
}

type CurrencyLimits struct {
	MaxMonthlyIncrease *float64 `yaml:"max_monthly_increase,omitempty" json:"max_monthly_increase,omitempty"`
	MaxTotal           *float64 `yaml:"max_total,omitempty" json:"max_total,omitempty"`
}

func Parse(r io.Reader) (Config, error) {
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)

	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode policy: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Config{}, fmt.Errorf("decode trailing policy document: %w", err)
		}
		return Config{}, fmt.Errorf("policy must contain exactly one YAML document")
	}

	return Validate(config)
}

func Validate(config Config) (Config, error) {
	if config.Version == 0 {
		return Config{}, fmt.Errorf("policy version is required")
	}
	if config.Version != CurrentVersion {
		return Config{}, fmt.Errorf("unsupported policy version %d", config.Version)
	}

	normalized := Config{
		Version:       config.Version,
		Limits:        make(map[string]CurrencyLimits, len(config.Limits)),
		FailOnSkipped: cloneBool(config.FailOnSkipped),
		MinCoverage:   cloneFloat(config.MinCoverage),
	}

	active := false
	for rawCurrency, rawLimits := range config.Limits {
		currency := normalizeCurrency(rawCurrency)
		if currency == "" {
			return Config{}, fmt.Errorf("policy currency must not be blank")
		}
		if _, exists := normalized.Limits[currency]; exists {
			return Config{}, fmt.Errorf("duplicate normalized currency %q", currency)
		}
		if rawLimits.MaxMonthlyIncrease == nil && rawLimits.MaxTotal == nil {
			return Config{}, fmt.Errorf("currency %s has no limits", currency)
		}

		limits := CurrencyLimits{
			MaxMonthlyIncrease: cloneFloat(rawLimits.MaxMonthlyIncrease),
			MaxTotal:           cloneFloat(rawLimits.MaxTotal),
		}
		if err := validateNonNegativeFinite("max_monthly_increase", limits.MaxMonthlyIncrease); err != nil {
			return Config{}, fmt.Errorf("currency %s: %w", currency, err)
		}
		if err := validateNonNegativeFinite("max_total", limits.MaxTotal); err != nil {
			return Config{}, fmt.Errorf("currency %s: %w", currency, err)
		}
		normalized.Limits[currency] = limits
		active = true
	}

	if normalized.FailOnSkipped != nil && *normalized.FailOnSkipped {
		active = true
	}
	if normalized.MinCoverage != nil {
		coverage := *normalized.MinCoverage
		if math.IsNaN(coverage) || math.IsInf(coverage, 0) || coverage < 0 || coverage > 1 {
			return Config{}, fmt.Errorf("min_coverage must be finite and within [0,1]")
		}
		if coverage > 0 {
			active = true
		}
	}
	if !active {
		return Config{}, fmt.Errorf("policy must contain at least one active rule")
	}

	return normalized, nil
}

func validateNonNegativeFinite(name string, value *float64) error {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
		return fmt.Errorf("%s must be finite and non-negative", name)
	}
	return nil
}

func normalizeCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
