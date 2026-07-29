package policy

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/susunola/cloudtab/internal/output"
)

func number(v float64) *float64 { return &v }
func boolean(v bool) *bool      { return &v }

func report(resources []output.ResourceCost, skipped int) output.Report {
	r := output.Report{Resources: resources}
	for i := 0; i < skipped; i++ {
		r.Skipped = append(r.Skipped, output.SkippedResource{Address: "skipped"})
	}
	return r
}

func resource(address string, components ...output.CostComponent) output.ResourceCost {
	return output.ResourceCost{Address: address, Components: components}
}

func component(currency string, monthly float64) output.CostComponent {
	return output.CostComponent{Currency: currency, MonthlyCost: monthly}
}

func requireViolationError(t *testing.T, err error, result Result) *ViolationError {
	t.Helper()
	var violationErr *ViolationError
	if !errors.As(err, &violationErr) {
		t.Fatalf("error = %T %v, want *ViolationError", err, err)
	}
	if violationErr.ExitCode() != 2 {
		t.Fatalf("ExitCode = %d, want 2", violationErr.ExitCode())
	}
	if !reflect.DeepEqual(violationErr.Result, result) {
		t.Fatalf("ViolationError.Result = %#v, want %#v", violationErr.Result, result)
	}
	return violationErr
}

func TestEvaluateBreakdownReturnsOrderedViolations(t *testing.T) {
	cfg := Config{
		Version:       1,
		Limits:        map[string]CurrencyLimits{"USD": {MaxTotal: number(100), MaxMonthlyIncrease: number(10)}},
		FailOnSkipped: boolean(true),
		MinCoverage:   number(0.75),
	}
	current := report([]output.ResourceCost{
		resource("priced", component("USD", 60), component("usd", 41)),
	}, 1)

	result, err := EvaluateBreakdown(cfg, current)
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want %q", result.Status, StatusFail)
	}
	requireViolationError(t, err, result)

	want := []struct {
		rule, scope, currency string
		actual, limit         *float64
	}{
		{RuleMaxTotal, ScopeCurrent, "USD", number(101), number(100)},
		{RuleMaxMonthlyIncrease, ScopeCurrent, "USD", nil, number(10)},
		{RuleFailOnSkipped, ScopeCurrent, "", number(1), number(0)},
		{RuleMinCoverage, ScopeCurrent, "", number(0.5), number(0.75)},
	}
	if len(result.Violations) != len(want) {
		t.Fatalf("Violations = %#v, want %d", result.Violations, len(want))
	}
	for i, expected := range want {
		got := result.Violations[i]
		if got.Rule != expected.rule || got.Scope != expected.scope || got.Currency != expected.currency ||
			!reflect.DeepEqual(got.Actual, expected.actual) || !reflect.DeepEqual(got.Limit, expected.limit) || got.Message == "" {
			t.Errorf("Violation[%d] = %#v, want rule=%q scope=%q currency=%q actual=%v limit=%v and message", i, got, expected.rule, expected.scope, expected.currency, expected.actual, expected.limit)
		}
	}

	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("Marshal Result: %v", marshalErr)
	}
	for _, field := range []string{`"status":"fail"`, `"violations"`, `"rule":"max_total"`, `"scope":"current"`, `"currency":"USD"`, `"actual":101`, `"limit":100`, `"message"`} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("JSON %s missing %s", encoded, field)
		}
	}
}

func TestEvaluateBreakdownAggregatesEachCurrencyWithoutFX(t *testing.T) {
	cfg := Config{Version: 1, Limits: map[string]CurrencyLimits{
		"USD": {MaxTotal: number(100)},
		"CNY": {MaxTotal: number(100)},
	}}
	current := report([]output.ResourceCost{
		resource("mixed", component("USD", 80), component("CNY", 80)),
	}, 0)

	result, err := EvaluateBreakdown(cfg, current)
	if err != nil || result.Status != StatusPass || len(result.Violations) != 0 {
		t.Fatalf("EvaluateBreakdown = (%#v, %v), want pass", result, err)
	}
}

func TestEvaluateDiffUsesAfterForTotalAndPerCurrencyDeltaForIncrease(t *testing.T) {
	cfg := Config{Version: 1, Limits: map[string]CurrencyLimits{
		"USD": {MaxTotal: number(100), MaxMonthlyIncrease: number(20)},
		"CNY": {MaxTotal: number(1000), MaxMonthlyIncrease: number(50)},
	}}
	before := report([]output.ResourceCost{
		resource("before", component("USD", 90), component("CNY", 200)),
	}, 0)
	after := report([]output.ResourceCost{
		resource("after", component("usd", 115), component("CNY", 190)),
	}, 0)

	result, err := EvaluateDiff(cfg, before, after)
	requireViolationError(t, err, result)
	if len(result.Violations) != 2 {
		t.Fatalf("Violations = %#v, want 2", result.Violations)
	}
	assertViolation(t, result.Violations[0], RuleMaxTotal, ScopeAfter, "USD", 115, 100)
	assertViolation(t, result.Violations[1], RuleMaxMonthlyIncrease, ScopeDiff, "USD", 25, 20)
}

func TestPolicyThresholdsIgnoreFloatRepresentationNoise(t *testing.T) {
	breakdownConfig := Config{Version: 1, Limits: map[string]CurrencyLimits{
		"USD": {MaxTotal: number(0.3)},
	}}
	breakdown := report([]output.ResourceCost{resource("x", component("USD", 0.1), component("USD", 0.2))}, 0)
	if result, err := EvaluateBreakdown(breakdownConfig, breakdown); err != nil || result.Status != StatusPass {
		t.Fatalf("0.1 + 0.2 should pass max_total 0.3: result=%#v err=%v", result, err)
	}

	diffConfig := Config{Version: 1, Limits: map[string]CurrencyLimits{
		"USD": {MaxMonthlyIncrease: number(0.3)},
	}}
	after := report([]output.ResourceCost{resource("x", component("USD", 0.1), component("USD", 0.2))}, 0)
	if result, err := EvaluateDiff(diffConfig, output.Report{}, after); err != nil || result.Status != StatusPass {
		t.Fatalf("0.1 + 0.2 should pass max_monthly_increase 0.3: result=%#v err=%v", result, err)
	}

	after = report([]output.ResourceCost{resource("x", component("USD", 0.31))}, 0)
	if result, err := EvaluateDiff(diffConfig, output.Report{}, after); err == nil || result.Status != StatusFail {
		t.Fatalf("0.31 should fail max_monthly_increase 0.3: result=%#v err=%v", result, err)
	}

	largeBaselineConfig := Config{Version: 1, Limits: map[string]CurrencyLimits{
		"USD": {MaxMonthlyIncrease: number(0.01)},
	}}
	before := report([]output.ResourceCost{resource("x", component("USD", 100_000_000.00))}, 0)
	after = report([]output.ResourceCost{resource("x", component("USD", 100_000_000.01))}, 0)
	if result, err := EvaluateDiff(largeBaselineConfig, before, after); err != nil || result.Status != StatusPass {
		t.Fatalf("exact 0.01 increase on large baseline should pass: result=%#v err=%v", result, err)
	}
	after = report([]output.ResourceCost{resource("x", component("USD", 100_000_000.02))}, 0)
	if result, err := EvaluateDiff(largeBaselineConfig, before, after); err == nil || result.Status != StatusFail {
		t.Fatalf("meaningful 0.02 increase should fail 0.01 limit: result=%#v err=%v", result, err)
	}
}

func TestEvaluateDiffDoesNotApplyTotalToBefore(t *testing.T) {
	cfg := Config{Version: 1, Limits: map[string]CurrencyLimits{"USD": {MaxTotal: number(100)}}}
	before := report([]output.ResourceCost{resource("before", component("USD", 150))}, 0)
	after := report([]output.ResourceCost{resource("after", component("USD", 90))}, 0)

	result, err := EvaluateDiff(cfg, before, after)
	if err != nil || result.Status != StatusPass {
		t.Fatalf("EvaluateDiff = (%#v, %v), want pass based on after total", result, err)
	}
}

func TestEvaluateDiffTreatsMissingCurrenciesAsZero(t *testing.T) {
	cfg := Config{Version: 1, Limits: map[string]CurrencyLimits{"USD": {MaxMonthlyIncrease: number(0)}}}
	before := report([]output.ResourceCost{resource("before", component("USD", 10))}, 0)

	result, err := EvaluateDiff(cfg, before, output.Report{})
	if err != nil || result.Status != StatusPass {
		t.Fatalf("EvaluateDiff = (%#v, %v), want pass for delta -10", result, err)
	}
}

func TestEvaluateDiffChecksSkippedAndCoverageForEachSide(t *testing.T) {
	cfg := Config{Version: 1, FailOnSkipped: boolean(true), MinCoverage: number(0.8)}
	before := report([]output.ResourceCost{resource("before", component("USD", 1))}, 1)
	after := report([]output.ResourceCost{
		resource("after-1", component("USD", 1)),
		resource("after-2", component("USD", 1)),
		resource("after-3", component("USD", 1)),
	}, 1)

	result, err := EvaluateDiff(cfg, before, after)
	requireViolationError(t, err, result)
	want := []struct{ rule, scope string }{
		{RuleFailOnSkipped, ScopeBefore},
		{RuleFailOnSkipped, ScopeAfter},
		{RuleMinCoverage, ScopeBefore},
		{RuleMinCoverage, ScopeAfter},
	}
	if len(result.Violations) != len(want) {
		t.Fatalf("Violations = %#v, want %d", result.Violations, len(want))
	}
	for i := range want {
		if result.Violations[i].Rule != want[i].rule || result.Violations[i].Scope != want[i].scope {
			t.Errorf("Violation[%d] = %#v, want rule=%q scope=%q", i, result.Violations[i], want[i].rule, want[i].scope)
		}
	}
	if *result.Violations[2].Actual != 0.5 || *result.Violations[3].Actual != 0.75 {
		t.Fatalf("coverage violations = %#v, want before=.5 after=.75", result.Violations[2:])
	}
}

func TestEvaluateCoverageOfEmptyReportIsOne(t *testing.T) {
	cfg := Config{Version: 1, MinCoverage: number(1)}
	for name, evaluate := range map[string]func() (Result, error){
		"breakdown": func() (Result, error) { return EvaluateBreakdown(cfg, output.Report{}) },
		"diff":      func() (Result, error) { return EvaluateDiff(cfg, output.Report{}, output.Report{}) },
	} {
		t.Run(name, func(t *testing.T) {
			result, err := evaluate()
			if err != nil || result.Status != StatusPass {
				t.Fatalf("evaluation = (%#v, %v), want pass", result, err)
			}
		})
	}
}

func TestEvaluationRejectsInvalidRawComponents(t *testing.T) {
	tests := map[string]output.Report{
		"blank currency on positive cost": report([]output.ResourceCost{resource("x", component("", 1))}, 0),
		"blank currency on negative cost": report([]output.ResourceCost{resource("x", component(" ", -1))}, 0),
		"nan":                             report([]output.ResourceCost{resource("x", component("USD", math.NaN()))}, 0),
		"positive infinity":               report([]output.ResourceCost{resource("x", component("USD", math.Inf(1)))}, 0),
		"negative infinity":               report([]output.ResourceCost{resource("x", component("USD", math.Inf(-1)))}, 0),
		"aggregate overflow": report([]output.ResourceCost{resource("x",
			component("USD", 1e308), component("USD", 1e308),
		)}, 0),
	}
	cfg := Config{Version: 1, Limits: map[string]CurrencyLimits{"USD": {MaxTotal: number(1)}}}
	for name, invalid := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := EvaluateBreakdown(cfg, invalid); err == nil {
				t.Fatal("EvaluateBreakdown accepted an invalid component")
			} else {
				var violationErr *ViolationError
				if errors.As(err, &violationErr) {
					t.Fatalf("invalid input returned policy violation: %v", err)
				}
			}
		})
	}
}

func TestEvaluationAllowsZeroCostComponentWithoutCurrency(t *testing.T) {
	cfg := Config{Version: 1, Limits: map[string]CurrencyLimits{"USD": {MaxTotal: number(0)}}}
	current := report([]output.ResourceCost{resource("placeholder", component("", 0))}, 0)
	result, err := EvaluateBreakdown(cfg, current)
	if err != nil || result.Status != StatusPass {
		t.Fatalf("EvaluateBreakdown = (%#v, %v), want pass", result, err)
	}
}

func TestEvaluateDiffIdentifiesInvalidScope(t *testing.T) {
	cfg := Config{Version: 1, Limits: map[string]CurrencyLimits{"USD": {MaxTotal: number(1)}}}
	invalidBefore := report([]output.ResourceCost{resource("x", component("", 1))}, 0)
	if _, err := EvaluateDiff(cfg, invalidBefore, output.Report{}); err == nil || !strings.Contains(err.Error(), ScopeBefore) {
		t.Fatalf("before error = %v, want scope", err)
	}
	invalidAfter := report([]output.ResourceCost{resource("x", component("", 1))}, 0)
	if _, err := EvaluateDiff(cfg, output.Report{}, invalidAfter); err == nil || !strings.Contains(err.Error(), ScopeAfter) {
		t.Fatalf("after error = %v, want scope", err)
	}
}

func TestViolationsAreDeterministicByRuleScopeAndCurrency(t *testing.T) {
	cfgA := Config{Version: 1, Limits: map[string]CurrencyLimits{
		"USD": {MaxTotal: number(1), MaxMonthlyIncrease: number(1)},
		"CNY": {MaxTotal: number(1), MaxMonthlyIncrease: number(1)},
	}}
	cfgB := Config{Version: 1, Limits: map[string]CurrencyLimits{
		"CNY": {MaxMonthlyIncrease: number(1), MaxTotal: number(1)},
		"USD": {MaxMonthlyIncrease: number(1), MaxTotal: number(1)},
	}}
	before := report([]output.ResourceCost{resource("before", component("USD", 0), component("CNY", 0))}, 0)
	afterA := report([]output.ResourceCost{resource("after", component("USD", 3), component("CNY", 3))}, 0)
	afterB := report([]output.ResourceCost{resource("after", component("CNY", 3), component("USD", 3))}, 0)

	resultA, _ := EvaluateDiff(cfgA, before, afterA)
	resultB, _ := EvaluateDiff(cfgB, before, afterB)
	if !reflect.DeepEqual(resultA, resultB) {
		t.Fatalf("results differ by map/component order:\nA=%#v\nB=%#v", resultA, resultB)
	}
	want := []string{"max_total:CNY", "max_total:USD", "max_monthly_increase:CNY", "max_monthly_increase:USD"}
	for i, expected := range want {
		got := resultA.Violations[i].Rule + ":" + resultA.Violations[i].Currency
		if got != expected {
			t.Errorf("Violation[%d] = %s, want %s", i, got, expected)
		}
	}
}

func TestEvaluationValidatesProgrammaticConfig(t *testing.T) {
	cfg := Config{Version: 2, FailOnSkipped: boolean(true)}
	if _, err := EvaluateBreakdown(cfg, output.Report{}); err == nil {
		t.Fatal("EvaluateBreakdown accepted unsupported config version")
	}
}

func assertViolation(t *testing.T, got Violation, rule, scope, currency string, actual, limit float64) {
	t.Helper()
	if got.Rule != rule || got.Scope != scope || got.Currency != currency || got.Actual == nil || *got.Actual != actual || got.Limit == nil || *got.Limit != limit || got.Message == "" {
		t.Fatalf("Violation = %#v, want rule=%q scope=%q currency=%q actual=%v limit=%v", got, rule, scope, currency, actual, limit)
	}
}
