package policy

import (
	"fmt"
	"math"
	"sort"

	"github.com/susunola/cloudtab/internal/output"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
)

const (
	RuleMaxTotal           = "max_total"
	RuleMaxMonthlyIncrease = "max_monthly_increase"
	RuleFailOnSkipped      = "fail_on_skipped"
	RuleMinCoverage        = "min_coverage"
)

const (
	ScopeCurrent = "current"
	ScopeBefore  = "before"
	ScopeAfter   = "after"
	ScopeDiff    = "diff"
)

type Result struct {
	Status     Status      `json:"status"`
	Violations []Violation `json:"violations"`
}

type Violation struct {
	Rule     string   `json:"rule"`
	Scope    string   `json:"scope"`
	Currency string   `json:"currency,omitempty"`
	Actual   *float64 `json:"actual,omitempty"`
	Limit    *float64 `json:"limit,omitempty"`
	Message  string   `json:"message"`
}

type ViolationError struct {
	Result Result
}

func (e *ViolationError) Error() string {
	return fmt.Sprintf("policy failed with %d violation(s)", len(e.Result.Violations))
}

func (e *ViolationError) ExitCode() int { return 2 }

func EvaluateBreakdown(config Config, current output.Report) (Result, error) {
	normalized, err := Validate(config)
	if err != nil {
		return Result{}, fmt.Errorf("validate policy: %w", err)
	}
	currentTotals, err := aggregate(current)
	if err != nil {
		return Result{}, fmt.Errorf("%s report: %w", ScopeCurrent, err)
	}

	violations := make([]Violation, 0)
	violations = appendMaxTotalViolations(violations, normalized, ScopeCurrent, currentTotals)
	violations = appendBaselineRequiredViolations(violations, normalized)
	violations = appendSkippedViolations(violations, normalized, ScopeCurrent, current)
	violations = appendCoverageViolations(violations, normalized, ScopeCurrent, current)
	return result(violations)
}

func EvaluateDiff(config Config, before, after output.Report) (Result, error) {
	normalized, err := Validate(config)
	if err != nil {
		return Result{}, fmt.Errorf("validate policy: %w", err)
	}
	beforeTotals, err := aggregate(before)
	if err != nil {
		return Result{}, fmt.Errorf("%s report: %w", ScopeBefore, err)
	}
	afterTotals, err := aggregate(after)
	if err != nil {
		return Result{}, fmt.Errorf("%s report: %w", ScopeAfter, err)
	}

	violations := make([]Violation, 0)
	violations = appendMaxTotalViolations(violations, normalized, ScopeAfter, afterTotals)
	violations, err = appendIncreaseViolations(violations, normalized, beforeTotals, afterTotals)
	if err != nil {
		return Result{}, err
	}
	violations = appendSkippedViolations(violations, normalized, ScopeBefore, before)
	violations = appendSkippedViolations(violations, normalized, ScopeAfter, after)
	violations = appendCoverageViolations(violations, normalized, ScopeBefore, before)
	violations = appendCoverageViolations(violations, normalized, ScopeAfter, after)
	return result(violations)
}

func result(violations []Violation) (Result, error) {
	status := StatusPass
	if len(violations) > 0 {
		status = StatusFail
	}
	result := Result{Status: status, Violations: violations}
	if status == StatusFail {
		return result, &ViolationError{Result: result}
	}
	return result, nil
}

func aggregate(report output.Report) (map[string]float64, error) {
	totals := make(map[string]float64)
	for _, resource := range report.Resources {
		for _, component := range resource.Components {
			value := component.MonthlyCost
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("resource %q has a non-finite monthly cost", resource.Address)
			}
			currency := normalizeCurrency(component.Currency)
			if currency == "" {
				if value != 0 {
					return nil, fmt.Errorf("resource %q has a non-zero monthly cost with blank currency", resource.Address)
				}
				continue
			}
			total := totals[currency] + value
			if math.IsNaN(total) || math.IsInf(total, 0) {
				return nil, fmt.Errorf("currency %s monthly cost aggregate is non-finite", currency)
			}
			totals[currency] = total
		}
	}
	return totals, nil
}

func exceedsAtScale(actual, limit, scale float64) bool {
	// Pricing arithmetic is float64. Ignore representation and cancellation
	// noise while retaining cent-level sensitivity even for very large totals.
	scale = math.Max(scale, math.Max(math.Abs(actual), math.Abs(limit)))
	tolerance := math.Max(1e-9, scale*1e-14)
	return actual-limit > tolerance
}

func exceeds(actual, limit float64) bool {
	return exceedsAtScale(actual, limit, 0)
}

func appendMaxTotalViolations(violations []Violation, config Config, scope string, totals map[string]float64) []Violation {
	for _, currency := range sortedCurrencies(config, func(limits CurrencyLimits) bool { return limits.MaxTotal != nil }) {
		limit := *config.Limits[currency].MaxTotal
		actual := totals[currency]
		if exceeds(actual, limit) {
			violations = append(violations, Violation{
				Rule: RuleMaxTotal, Scope: scope, Currency: currency,
				Actual: floatPointer(actual), Limit: floatPointer(limit),
				Message: fmt.Sprintf("%s total %g exceeds limit %g", currency, actual, limit),
			})
		}
	}
	return violations
}

func appendBaselineRequiredViolations(violations []Violation, config Config) []Violation {
	for _, currency := range sortedCurrencies(config, func(limits CurrencyLimits) bool { return limits.MaxMonthlyIncrease != nil }) {
		limit := *config.Limits[currency].MaxMonthlyIncrease
		violations = append(violations, Violation{
			Rule: RuleMaxMonthlyIncrease, Scope: ScopeCurrent, Currency: currency,
			Limit: floatPointer(limit), Message: "max_monthly_increase requires a before report",
		})
	}
	return violations
}

func appendIncreaseViolations(violations []Violation, config Config, before, after map[string]float64) ([]Violation, error) {
	for _, currency := range sortedCurrencies(config, func(limits CurrencyLimits) bool { return limits.MaxMonthlyIncrease != nil }) {
		limit := *config.Limits[currency].MaxMonthlyIncrease
		beforeTotal, afterTotal := before[currency], after[currency]
		actual := afterTotal - beforeTotal
		if math.IsNaN(actual) || math.IsInf(actual, 0) {
			return nil, fmt.Errorf("%s report: currency %s monthly increase is non-finite", ScopeDiff, currency)
		}
		operandScale := math.Max(math.Abs(beforeTotal), math.Abs(afterTotal))
		if exceedsAtScale(actual, limit, operandScale) {
			violations = append(violations, Violation{
				Rule: RuleMaxMonthlyIncrease, Scope: ScopeDiff, Currency: currency,
				Actual: floatPointer(actual), Limit: floatPointer(limit),
				Message: fmt.Sprintf("%s monthly increase %g exceeds limit %g", currency, actual, limit),
			})
		}
	}
	return violations, nil
}

func appendSkippedViolations(violations []Violation, config Config, scope string, report output.Report) []Violation {
	if config.FailOnSkipped == nil || !*config.FailOnSkipped || len(report.Skipped) == 0 {
		return violations
	}
	actual := float64(len(report.Skipped))
	return append(violations, Violation{
		Rule: RuleFailOnSkipped, Scope: scope,
		Actual: floatPointer(actual), Limit: floatPointer(0),
		Message: fmt.Sprintf("%s report has %d skipped resource(s)", scope, len(report.Skipped)),
	})
}

func appendCoverageViolations(violations []Violation, config Config, scope string, report output.Report) []Violation {
	if config.MinCoverage == nil {
		return violations
	}
	limit := *config.MinCoverage
	actual := coverage(report)
	if actual >= limit {
		return violations
	}
	return append(violations, Violation{
		Rule: RuleMinCoverage, Scope: scope,
		Actual: floatPointer(actual), Limit: floatPointer(limit),
		Message: fmt.Sprintf("%s coverage %g is below minimum %g", scope, actual, limit),
	})
}

func coverage(report output.Report) float64 {
	priced := len(report.Resources)
	total := priced + len(report.Skipped)
	if total == 0 {
		return 1
	}
	return float64(priced) / float64(total)
}

func sortedCurrencies(config Config, include func(CurrencyLimits) bool) []string {
	currencies := make([]string, 0, len(config.Limits))
	for currency, limits := range config.Limits {
		if include(limits) {
			currencies = append(currencies, currency)
		}
	}
	sort.Strings(currencies)
	return currencies
}

func floatPointer(value float64) *float64 {
	copy := value
	return &copy
}
