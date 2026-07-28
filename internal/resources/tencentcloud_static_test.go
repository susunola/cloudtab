package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

// TestStaticUsageMappersRegistered verifies the four usage-driven Tencent
// resources are registered as StaticMappers and produce a zero-cost,
// currency-agnostic placeholder with a non-empty note (never a fabricated
// price). Currency is empty on purpose so the $0 note does not force a "mixed
// currencies" verdict on a Tencent International (USD) plan.
func TestStaticUsageMappersRegistered(t *testing.T) {
	cases := []struct {
		tfType string
	}{
		{"tencentcloud_cos_bucket"},
		{"tencentcloud_cdn_domain"},
		{"tencentcloud_cfs_file_system"},
		{"tencentcloud_scf_function"},
	}
	reg := DefaultRegistry()
	for _, c := range cases {
		m, ok := reg.Lookup(c.tfType)
		if !ok {
			t.Fatalf("%s not registered", c.tfType)
		}
		sm, ok := m.(StaticMapper)
		if !ok {
			t.Fatalf("%s type %T does not implement StaticMapper", c.tfType, m)
		}
		comps, err := sm.Estimate(parser.PlannedResource{})
		if err != nil {
			t.Fatalf("%s Estimate error: %v", c.tfType, err)
		}
		if len(comps) != 1 {
			t.Fatalf("%s Estimate returned %d components, want 1", c.tfType, len(comps))
		}
		if comps[0].MonthlyCost != 0 || comps[0].Currency != "" {
			t.Errorf("%s component = %+v, want zero-cost currency-agnostic placeholder", c.tfType, comps[0])
		}
		if comps[0].Name == "" {
			t.Errorf("%s component has empty note", c.tfType)
		}
	}
}
