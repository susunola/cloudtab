package main

import (
	"os"
	"path/filepath"
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

func TestEngineCreationWithCacheDir(t *testing.T) {
	// Ensure newEngine does not fail due to cache dir when env creds are missing.
	os.Unsetenv("TENCENTCLOUD_SECRET_ID")
	os.Unsetenv("TENCENTCLOUD_SECRET_KEY")
	_, err := newEngine("ap-guangzhou", "", false, t.TempDir())
	if err == nil {
		t.Fatal("expected error without credentials")
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
