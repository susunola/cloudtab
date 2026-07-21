package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsageYAMLEmptyPath(t *testing.T) {
	u, err := LoadUsageYAML("")
	if err != nil {
		t.Fatalf("LoadUsageYAML(\"\") error = %v", err)
	}
	if len(u) != 0 {
		t.Fatalf("len = %d, want 0", len(u))
	}
}

func TestLoadUsageYAML(t *testing.T) {
	doc := `
 tencentcloud_cos_bucket.static_site:
   monthly_storage_gb: 200
   monthly_get_requests: 1000000
 tencentcloud_redis_instance.cache:
   mem_size: 4096
 `
	p := filepath.Join(t.TempDir(), "usage.yml")
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	u, err := LoadUsageYAML(p)
	if err != nil {
		t.Fatalf("LoadUsageYAML() error = %v", err)
	}
	if got := u["tencentcloud_cos_bucket.static_site"]["monthly_storage_gb"]; got != 200 {
		t.Fatalf("monthly_storage_gb = %v, want 200", got)
	}
	if got := u["tencentcloud_redis_instance.cache"]["mem_size"]; got != 4096 {
		t.Fatalf("mem_size = %v, want 4096", got)
	}
}

func TestLoadUsageYAMLBadPath(t *testing.T) {
	if _, err := LoadUsageYAML(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("expected error for missing usage file")
	}
}
