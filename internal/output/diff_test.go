package output

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func rcCur(addr, typ, currency string, monthly float64) ResourceCost {
	return ResourceCost{
		Address: addr,
		Type:    typ,
		Components: []CostComponent{
			{Name: "compute", MonthlyCost: monthly, Currency: currency},
		},
	}
}

// A diff spanning two currencies must NOT sum incomparable amounts into a single
// TOTAL; the table footer shows a dash and flags the mix instead.
func TestRenderDiffTableMixedCurrency(t *testing.T) {
	before := Report{Resources: []ResourceCost{
		rcCur("tencentcloud_instance.a", "tencentcloud_instance", "CNY", 100),
	}}
	after := Report{Resources: []ResourceCost{
		rcCur("tencentcloud_instance.a", "tencentcloud_instance", "CNY", 100),
		rcCur("aws_instance.b", "aws_instance", "USD", 20),
	}}
	d := ComputeDiff(before, after)
	if d.Currency != "" {
		t.Fatalf("Currency = %q, want empty (mixed)", d.Currency)
	}

	var buf bytes.Buffer
	if err := RenderDiff(&buf, d, "table"); err != nil {
		t.Fatalf("RenderDiff: %v", err)
	}
	// tablewriter uppercases footer text, so match case-insensitively.
	out := buf.String()
	if !strings.Contains(strings.ToLower(out), "mixed currencies") {
		t.Errorf("table footer should flag mixed currencies, got:\n%s", out)
	}
	// The USD amount (20) and CNY amount (100) must not be summed into 120.
	if strings.Contains(out, "120.00") {
		t.Errorf("mixed-currency amounts were summed (found 120.00):\n%s", out)
	}
}

// The markdown (PR-comment) renderer must follow the same rule as the table: a
// diff spanning two currencies must not present a single summed headline total.
func TestRenderDiffMarkdownMixedCurrency(t *testing.T) {
	before := Report{Resources: []ResourceCost{
		rcCur("tencentcloud_instance.a", "tencentcloud_instance", "CNY", 100),
	}}
	after := Report{Resources: []ResourceCost{
		rcCur("tencentcloud_instance.a", "tencentcloud_instance", "CNY", 100),
		rcCur("aws_instance.b", "aws_instance", "USD", 20),
	}}
	d := ComputeDiff(before, after)
	if d.Currency != "" {
		t.Fatalf("Currency = %q, want empty (mixed)", d.Currency)
	}

	var buf bytes.Buffer
	if err := RenderDiff(&buf, d, "markdown"); err != nil {
		t.Fatalf("RenderDiff: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "mixed currencies") {
		t.Errorf("markdown should flag mixed currencies, got:\n%s", out)
	}
	// The headline must not present the summed CNY+USD total (100 + 20 = 120).
	if strings.Contains(out, "120.00") {
		t.Errorf("markdown summed mixed-currency totals (found 120.00):\n%s", out)
	}
}

// The markdown skipped section must surface each resource's real skip reason
// (grouped by reason), never a blanket hardcoded "unsupported type" — a skip
// can be an auth failure, API error, parse failure, or panic.
func TestRenderDiffMarkdownSkippedReasons(t *testing.T) {
	before := Report{
		Skipped: []SkippedResource{
			{Address: "tencentcloud_instance.auth", Type: "tencentcloud_instance", Reason: "query x: AuthFailure: invalid secret id"},
			{Address: "aws_instance.unsup", Type: "aws_instance", Reason: "unsupported resource type"},
		},
	}
	after := Report{
		Skipped: []SkippedResource{
			{Address: "tencentcloud_instance.auth", Type: "tencentcloud_instance", Reason: "query x: AuthFailure: invalid secret id"},
			{Address: "aws_instance.unsup", Type: "aws_instance", Reason: "unsupported resource type"},
			{Address: "tencentcloud_cbs_storage.panic", Type: "tencentcloud_cbs_storage", Reason: "panic pricing tencentcloud_cbs_storage.x: boom"},
		},
	}
	d := ComputeDiff(before, after)

	var buf bytes.Buffer
	if err := RenderDiff(&buf, d, "markdown"); err != nil {
		t.Fatalf("RenderDiff: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"AuthFailure", "unsupported resource type", "panic pricing"} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown should surface reason %q, got:\n%s", want, out)
		}
	}
	// The old blanket hardcoded label must be gone.
	if strings.Contains(out, "skipped (unsupported type)") {
		t.Errorf("markdown still uses the hardcoded unsupported-type label:\n%s", out)
	}
}

func rc(addr, typ string, monthly float64) ResourceCost {
	return ResourceCost{
		Address: addr,
		Type:    typ,
		Components: []CostComponent{
			{Name: "compute", MonthlyCost: monthly, Currency: "CNY"},
		},
	}
}

func findDiff(d DiffReport, addr string) (ResourceDiff, bool) {
	for _, r := range d.Resources {
		if r.Address == addr {
			return r, true
		}
	}
	return ResourceDiff{}, false
}

func eq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestComputeDiff(t *testing.T) {
	before := Report{
		Resources: []ResourceCost{
			rc("tencentcloud_instance.keep", "tencentcloud_instance", 100), // unchanged
			rc("tencentcloud_instance.grow", "tencentcloud_instance", 50),  // changed up
			rc("tencentcloud_instance.gone", "tencentcloud_instance", 30),  // removed
		},
		Skipped: []SkippedResource{
			{Address: "aws_instance.old_skip", Type: "aws_instance", Reason: "unsupported instance type"},
		},
	}
	after := Report{
		Resources: []ResourceCost{
			rc("tencentcloud_instance.keep", "tencentcloud_instance", 100), // unchanged
			rc("tencentcloud_instance.grow", "tencentcloud_instance", 80),  // changed up
			rc("tencentcloud_instance.new", "tencentcloud_instance", 20),   // added
		},
		Skipped: []SkippedResource{
			{Address: "aws_instance.new_skip", Type: "aws_instance", Reason: "unsupported instance type"},
		},
	}

	d := ComputeDiff(before, after)

	// Currency inferred from both reports (both use CNY in the rc helper).
	if d.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", d.Currency)
	}

	// Skipped resources merged from both sides, de-duplicated.
	if len(d.Skipped) != 2 {
		t.Fatalf("Skipped = %d, want 2", len(d.Skipped))
	}
	gotAddr := map[string]bool{}
	for _, s := range d.Skipped {
		gotAddr[s.Address] = true
	}
	if !gotAddr["aws_instance.old_skip"] || !gotAddr["aws_instance.new_skip"] {
		t.Errorf("skipped addresses missing: %v", d.Skipped)
	}

	// Totals: before = 180, after = 200, delta = +20
	if !eq(d.BeforeTotal, 180) {
		t.Errorf("BeforeTotal = %v, want 180", d.BeforeTotal)
	}
	if !eq(d.AfterTotal, 200) {
		t.Errorf("AfterTotal = %v, want 200", d.AfterTotal)
	}
	if !eq(d.DeltaTotal, 20) {
		t.Errorf("DeltaTotal = %v, want 20", d.DeltaTotal)
	}

	// Unchanged
	if r, ok := findDiff(d, "tencentcloud_instance.keep"); !ok || r.Kind != DiffSame || !eq(r.DeltaMonthly, 0) {
		t.Errorf("keep: got %+v (ok=%v), want DiffSame delta 0", r, ok)
	}
	// Changed
	if r, ok := findDiff(d, "tencentcloud_instance.grow"); !ok || r.Kind != DiffChange || !eq(r.DeltaMonthly, 30) {
		t.Errorf("grow: got %+v (ok=%v), want DiffChange delta 30", r, ok)
	}
	// Removed
	if r, ok := findDiff(d, "tencentcloud_instance.gone"); !ok || r.Kind != DiffRemove || !eq(r.DeltaMonthly, -30) {
		t.Errorf("gone: got %+v (ok=%v), want DiffRemove delta -30", r, ok)
	}
	// Added
	if r, ok := findDiff(d, "tencentcloud_instance.new"); !ok || r.Kind != DiffAdd || !eq(r.DeltaMonthly, 20) {
		t.Errorf("new: got %+v (ok=%v), want DiffAdd delta 20", r, ok)
	}

	// Deterministic ordering (sorted by address).
	for i := 1; i < len(d.Resources); i++ {
		if d.Resources[i-1].Address > d.Resources[i].Address {
			t.Errorf("resources not sorted: %q before %q",
				d.Resources[i-1].Address, d.Resources[i].Address)
		}
	}
}

func TestComputeDiffEmpty(t *testing.T) {
	d := ComputeDiff(Report{}, Report{})
	if len(d.Resources) != 0 {
		t.Errorf("expected no resources, got %d", len(d.Resources))
	}
	if !eq(d.DeltaTotal, 0) {
		t.Errorf("DeltaTotal = %v, want 0", d.DeltaTotal)
	}
}
