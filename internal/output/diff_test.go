package output

import (
	"math"
	"testing"
)

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
	before := Report{Resources: []ResourceCost{
		rc("tencentcloud_instance.keep", "tencentcloud_instance", 100), // unchanged
		rc("tencentcloud_instance.grow", "tencentcloud_instance", 50),  // changed up
		rc("tencentcloud_instance.gone", "tencentcloud_instance", 30),  // removed
	}}
	after := Report{Resources: []ResourceCost{
		rc("tencentcloud_instance.keep", "tencentcloud_instance", 100), // unchanged
		rc("tencentcloud_instance.grow", "tencentcloud_instance", 80),  // changed up
		rc("tencentcloud_instance.new", "tencentcloud_instance", 20),   // added
	}}

	d := ComputeDiff(before, after)

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
