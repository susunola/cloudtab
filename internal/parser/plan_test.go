package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPlanJSON(t *testing.T) {
	// testdata/example.plan.json lives at the repo root; find it relative to
	// this package (internal/parser → ../../testdata).
	path := filepath.Join("..", "..", "testdata", "example.plan.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not found (%v); skipping", err)
	}

	p, err := LoadPlanJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanJSON: %v", err)
	}
	if len(p.Resources) != 1 {
		t.Fatalf("expected 1 costed resource, got %d", len(p.Resources))
	}
	r := p.Resources[0]
	if r.Address != "tencentcloud_instance.web" {
		t.Errorf("address = %q", r.Address)
	}
	if r.Type != "tencentcloud_instance" {
		t.Errorf("type = %q", r.Type)
	}
	// Region should fall back to the provider default when not set on the resource.
	if r.Region != "ap-shanghai" {
		t.Errorf("region = %q, want ap-shanghai (provider default)", r.Region)
	}
	if got, _ := r.After["instance_type"].(string); got != "S5.MEDIUM4" {
		t.Errorf("instance_type = %q, want S5.MEDIUM4", got)
	}
}

func TestContributesToCost(t *testing.T) {
	cases := []struct {
		actions []string
		want    bool
	}{
		{[]string{"create"}, true},
		{[]string{"update"}, true},
		{[]string{"create", "update"}, true},
		{[]string{"delete"}, false},
		{[]string{"no-op"}, false},
		{[]string{"delete", "create"}, true}, // replace: still costs
		{nil, false},
	}
	for _, c := range cases {
		if got := contributesToCost(c.actions); got != c.want {
			t.Errorf("contributesToCost(%v) = %v, want %v", c.actions, got, c.want)
		}
	}
}

func TestLoadPlanJSONBadPath(t *testing.T) {
	if _, err := LoadPlanJSON(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestLoadPlanJSONActions verifies that only create/update contribute to cost,
// that each resource is emitted exactly once (no double-count for update — the
// P1-5 concern), and that delete/no-op are excluded (delete is handled by diff
// mode, not the cost pass).
func TestLoadPlanJSONActions(t *testing.T) {
	doc := `{
      "format_version": "1.2",
      "configuration": {"provider_config": {"tencentcloud": {"expressions": {"region": {"constant_value": "ap-guangzhou"}}}}},
      "resource_changes": [
        {"address": "tencentcloud_instance.web", "type": "tencentcloud_instance", "name": "web",
         "change": {"actions": ["update"], "after": {"instance_type": "S5.LARGE8"}}},
        {"address": "tencentcloud_cbs_storage.data", "type": "tencentcloud_cbs_storage", "name": "data",
         "change": {"actions": ["delete"], "after": null}},
        {"address": "tencentcloud_instance.idle", "type": "tencentcloud_instance", "name": "idle",
         "change": {"actions": ["no-op"], "after": {"instance_type": "S5.SMALL2"}}}
      ]
    }`
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPlanJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanJSON: %v", err)
	}
	// Only the update should survive; delete + no-op are dropped.
	if len(p.Resources) != 1 {
		t.Fatalf("expected 1 costed resource (the update), got %d: %+v", len(p.Resources), p.Resources)
	}
	// Exactly once — no double-count of the updated resource.
	seen := map[string]int{}
	for _, r := range p.Resources {
		seen[r.Address]++
	}
	if seen["tencentcloud_instance.web"] != 1 {
		t.Errorf("updated resource counted %d times, want 1", seen["tencentcloud_instance.web"])
	}
	if p.Resources[0].Region != "ap-guangzhou" {
		t.Errorf("region = %q, want ap-guangzhou", p.Resources[0].Region)
	}
}
