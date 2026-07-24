package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
	"github.com/susunola/cloudtab/internal/resources"
)

// panicMapper is a StaticMapper whose Estimate always panics, used to prove a
// misbehaving mapper cannot crash the whole pricing run.
type panicMapper struct{}

func (panicMapper) Extract(parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{}, nil
}
func (panicMapper) Parse(pricing.PriceRequest, []byte) ([]output.CostComponent, error) {
	return nil, nil
}
func (panicMapper) Estimate(parser.PlannedResource) ([]output.CostComponent, error) {
	panic("boom: simulated mapper panic")
}

func TestPriceJobRecoversFromPanic(t *testing.T) {
	reg := resources.NewRegistry()
	reg.Register("panic_resource", panicMapper{})
	r := parser.PlannedResource{Address: "panic_resource.x", Type: "panic_resource"}

	// failOnError=false: a panic must degrade to a SkippedResource, not crash.
	got := priceJob(nil, reg, r, false)
	if got.err != nil {
		t.Fatalf("failOnError=false: got err %v, want nil (should soft-skip)", got.err)
	}
	if got.skip == nil {
		t.Fatal("failOnError=false: expected a SkippedResource, got none")
	}
	if got.skip.Address != r.Address {
		t.Errorf("skip.Address = %q, want %q", got.skip.Address, r.Address)
	}

	// failOnError=true: a panic must surface as an error, still no crash.
	got = priceJob(nil, reg, r, true)
	if got.err == nil {
		t.Fatal("failOnError=true: expected an error, got nil")
	}
}

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

func TestCachePathForFlags(t *testing.T) {
	if got := cachePathForFlags(true, ""); got != "" {
		t.Fatalf("no-cache: got %q, want empty", got)
	}
	if got := cachePathForFlags(false, "/tmp/ct"); got != filepath.Join("/tmp/ct", "cache.db") {
		t.Fatalf("custom dir: got %q, want %q", got, filepath.Join("/tmp/ct", "cache.db"))
	}
	// Default path is relative to $HOME; just assert it ends with the expected file.
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := cachePathForFlags(false, ""); got != filepath.Join(home, ".cloudtab", "cache.db") {
		t.Fatalf("default: got %q, want %q", got, filepath.Join(home, ".cloudtab", "cache.db"))
	}
}

func TestEngineCreationWithoutTencentCreds(t *testing.T) {
	// Tencent creds are validated lazily (only when a Tencent Cloud resource is
	// actually priced), so a multi-cloud plan must not require them up front.
	os.Unsetenv("TENCENTCLOUD_SECRET_ID")
	os.Unsetenv("TENCENTCLOUD_SECRET_KEY")
	eng, err := newEngine("ap-guangzhou", "", false, t.TempDir(), 0, 0, 0)
	if err != nil {
		t.Fatalf("newEngine without Tencent creds: %v (want no error)", err)
	}
	if eng == nil {
		t.Fatal("newEngine returned nil engine")
	}
}

func TestResolveConcurrency(t *testing.T) {
	// Positive flag wins over everything.
	t.Setenv("CLOUDTAB_CONCURRENCY", "3")
	if got := resolveConcurrency(5); got != 5 {
		t.Errorf("flag should win: got %d, want 5", got)
	}
	// Zero flag falls back to a valid env value.
	if got := resolveConcurrency(0); got != 3 {
		t.Errorf("env fallback: got %d, want 3", got)
	}
	// A non-positive / unparseable env value falls through to the default.
	t.Setenv("CLOUDTAB_CONCURRENCY", "0")
	if got := resolveConcurrency(0); got != defaultConcurrency {
		t.Errorf("bad env -> default: got %d, want %d", got, defaultConcurrency)
	}
	t.Setenv("CLOUDTAB_CONCURRENCY", "abc")
	if got := resolveConcurrency(0); got != defaultConcurrency {
		t.Errorf("unparseable env -> default: got %d, want %d", got, defaultConcurrency)
	}
	// Empty env + zero flag -> default.
	t.Setenv("CLOUDTAB_CONCURRENCY", "")
	if got := resolveConcurrency(0); got != defaultConcurrency {
		t.Errorf("empty -> default: got %d, want %d", got, defaultConcurrency)
	}
}

func TestResolveSite(t *testing.T) {
	// Flag wins over env.
	t.Setenv("TENCENTCLOUD_SITE", "domestic")
	if got := resolveSite("intl"); got != "intl" {
		t.Errorf("flag should win: got %q, want intl", got)
	}
	// Flag is whitespace-trimmed.
	if got := resolveSite("  intl  "); got != "intl" {
		t.Errorf("flag trim: got %q, want intl", got)
	}
	// Empty flag falls back to env.
	if got := resolveSite(""); got != "domestic" {
		t.Errorf("env fallback: got %q, want domestic", got)
	}
	// Empty flag + empty env -> empty (engine treats as domestic default).
	t.Setenv("TENCENTCLOUD_SITE", "")
	if got := resolveSite(""); got != "" {
		t.Errorf("default: got %q, want empty", got)
	}
}
