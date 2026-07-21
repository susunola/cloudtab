package main

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

func TestMergeUsageIntoAfter(t *testing.T) {
	orig := parser.PlannedResource{
		Address: "tencentcloud_redis_instance.cache",
		After: map[string]interface{}{
			"mem_size":    1024,
			"charge_type": "POSTPAID",
		},
	}
	usage := map[string]interface{}{
		"mem_size":         4096, // override
		"prepaid_period":   12,   // add
		"monthly_qps_peak": 5000,
	}

	merged := mergeUsageIntoAfter(orig, usage)

	if got := merged.After["mem_size"]; got != 4096 {
		t.Fatalf("mem_size = %v, want 4096", got)
	}
	if got := merged.After["charge_type"]; got != "POSTPAID" {
		t.Fatalf("charge_type = %v, want POSTPAID", got)
	}
	if got := merged.After["prepaid_period"]; got != 12 {
		t.Fatalf("prepaid_period = %v, want 12", got)
	}
	if got := merged.After["monthly_qps_peak"]; got != 5000 {
		t.Fatalf("monthly_qps_peak = %v, want 5000", got)
	}

	// Ensure original map is not mutated.
	if got := orig.After["mem_size"]; got != 1024 {
		t.Fatalf("orig.mem_size mutated to %v, want 1024", got)
	}
}
