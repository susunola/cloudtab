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

func TestProviderForType(t *testing.T) {
	cases := []struct {
		tfType, want string
	}{
		{"tencentcloud_instance", "tencentcloud"},
		{"tencentcloud_cbs_storage", "tencentcloud"},
		{"aws_instance", "aws"},
		{"aws_db_instance", "aws"},
		{"alicloud_instance", "alibaba"},
		{"alicloud_db_instance", "alibaba"},
		{"alicloud_kvstore_instance", "alibaba"},
		{"alicloud_vpn_gateway", "alibaba"},
		{"huaweicloud_compute_instance", "huawei"},
		{"huaweicloud_rds_instance", "huawei"},
		{"huaweicloud_cce_cluster", "huawei"},
		{"huaweicloud_evs_volume", "huawei"},
		{"some_unknown_type", "tencentcloud"},
		{"", "tencentcloud"},
	}
	for _, c := range cases {
		if got := ProviderForType(c.tfType); got != c.want {
			t.Errorf("ProviderForType(%q) = %q, want %q", c.tfType, got, c.want)
		}
	}
}

func TestLoadAlibabaPlanJSON(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "alibaba_plan.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not found (%v); skipping", err)
	}
	p, err := LoadPlanJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanJSON: %v", err)
	}
	if len(p.Resources) != 9 {
		t.Fatalf("expected 9 alibaba resources, got %d", len(p.Resources))
	}
	// Verify provider routing
	for _, r := range p.Resources {
		if ProviderForType(r.Type) != "alibaba" {
			t.Errorf("resource %q type %q should route to alibaba, got %q", r.Address, r.Type, ProviderForType(r.Type))
		}
	}
	// Verify ECS details
	ecs := p.Resources[0]
	if ecs.Region != "cn-hangzhou" {
		t.Errorf("alibaba ECS region = %q, want cn-hangzhou", ecs.Region)
	}
}

func TestLoadHuaweiPlanJSON(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "huawei_plan.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not found (%v); skipping", err)
	}
	p, err := LoadPlanJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanJSON: %v", err)
	}
	if len(p.Resources) != 9 {
		t.Fatalf("expected 9 huawei resources, got %d", len(p.Resources))
	}
	for _, r := range p.Resources {
		if ProviderForType(r.Type) != "huawei" {
			t.Errorf("resource %q type %q should route to huawei, got %q", r.Address, r.Type, ProviderForType(r.Type))
		}
	}
	ecs := p.Resources[0]
	if ecs.Region != "cn-north-4" {
		t.Errorf("huawei ECS region = %q, want cn-north-4", ecs.Region)
	}
}

// TestLoadPlanJSONAliasedProviderRegion pins item #20: a provider declared under
// an aliased block ("tencentcloud.guangzhou") must still contribute its region,
// otherwise its resources would silently fall back to the CLI --region.
func TestLoadPlanJSONAliasedProviderRegion(t *testing.T) {
	doc := `{
      "format_version": "1.2",
      "configuration": {"provider_config": {"tencentcloud.guangzhou": {"name": "tencentcloud", "expressions": {"region": {"constant_value": "ap-guangzhou-6"}}}}},
      "resource_changes": [
        {"address": "tencentcloud_instance.web", "type": "tencentcloud_instance", "name": "web",
         "change": {"actions": ["create"], "after": {"instance_type": "S5.LARGE8"}}}
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
	if len(p.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(p.Resources))
	}
	if p.Resources[0].Region != "ap-guangzhou-6" {
		t.Errorf("aliased provider region = %q, want ap-guangzhou-6", p.Resources[0].Region)
	}
}

func writePlanJSON(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func resourceAddresses(resources []PlannedResource) map[string]bool {
	addresses := make(map[string]bool, len(resources))
	for _, resource := range resources {
		addresses[resource.Address] = true
	}
	return addresses
}

func TestLoadPlanInventoryJSONUsesFinalManagedResources(t *testing.T) {
	path := writePlanJSON(t, `{
      "format_version": "1.2",
      "planned_values": {
        "root_module": {
          "resources": [
            {"address": "tencentcloud_instance.noop", "mode": "managed", "type": "tencentcloud_instance", "name": "noop", "values": {"instance_type": "S5.SMALL2"}},
            {"address": "tencentcloud_instance.update", "mode": "managed", "type": "tencentcloud_instance", "name": "update", "values": {"instance_type": "S5.LARGE8"}},
            {"address": "tencentcloud_instance.create", "mode": "managed", "type": "tencentcloud_instance", "name": "create", "values": {"instance_type": "S5.MEDIUM4"}},
            {"address": "data.tencentcloud_images.base", "mode": "data", "type": "tencentcloud_images", "name": "base", "values": {"image_type": "PUBLIC_IMAGE"}}
          ],
          "child_modules": [{
            "address": "module.workers[0]",
            "resources": [
              {"address": "module.workers[0].tencentcloud_instance.child", "mode": "managed", "type": "tencentcloud_instance", "name": "child", "values": {"instance_type": "S5.SMALL2"}},
              {"address": "module.workers[0].data.tencentcloud_images.base", "mode": "data", "type": "tencentcloud_images", "name": "base", "values": {}}
            ]
          }]
        }
      },
      "resource_changes": [
        {"address": "tencentcloud_instance.noop", "type": "tencentcloud_instance", "name": "noop", "change": {"actions": ["no-op"], "after": {"instance_type": "S5.SMALL2"}}},
        {"address": "tencentcloud_instance.update", "type": "tencentcloud_instance", "name": "update", "change": {"actions": ["update"], "after": {"instance_type": "S5.LARGE8"}}},
        {"address": "tencentcloud_instance.create", "type": "tencentcloud_instance", "name": "create", "change": {"actions": ["create"], "after": {"instance_type": "S5.MEDIUM4"}}},
        {"address": "tencentcloud_instance.delete", "type": "tencentcloud_instance", "name": "delete", "change": {"actions": ["delete"], "after": null}}
      ]
    }`)

	plan, err := LoadPlanInventoryJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanInventoryJSON: %v", err)
	}
	got := resourceAddresses(plan.Resources)
	for _, want := range []string{
		"tencentcloud_instance.noop",
		"tencentcloud_instance.update",
		"tencentcloud_instance.create",
		"module.workers[0].tencentcloud_instance.child",
	} {
		if !got[want] {
			t.Errorf("missing final managed resource %q: %v", want, got)
		}
	}
	for _, unwanted := range []string{
		"tencentcloud_instance.delete",
		"data.tencentcloud_images.base",
		"module.workers[0].data.tencentcloud_images.base",
	} {
		if got[unwanted] {
			t.Errorf("unexpected inventory resource %q: %v", unwanted, got)
		}
	}
	if len(plan.Resources) != 4 {
		t.Fatalf("inventory resource count = %d, want 4: %+v", len(plan.Resources), plan.Resources)
	}
}

func TestLoadPlanInventoryJSONPresentEmptyRootIsEmpty(t *testing.T) {
	path := writePlanJSON(t, `{
      "format_version": "1.2",
      "planned_values": {"root_module": {}},
      "resource_changes": [
        {"address": "tencentcloud_instance.destroyed", "type": "tencentcloud_instance", "name": "destroyed", "change": {"actions": ["delete"], "after": null}},
        {"address": "tencentcloud_instance.stale", "type": "tencentcloud_instance", "name": "stale", "change": {"actions": ["create"], "after": {"instance_type": "S5.SMALL2"}}}
      ]
    }`)

	plan, err := LoadPlanInventoryJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanInventoryJSON: %v", err)
	}
	if len(plan.Resources) != 0 {
		t.Fatalf("destroy-all inventory = %+v, want empty", plan.Resources)
	}
}

func TestLoadPlanInventoryJSONFallsBackWhenPlannedRootAbsent(t *testing.T) {
	path := writePlanJSON(t, `{
      "format_version": "1.2",
      "resource_changes": [
        {"address": "tencentcloud_instance.update", "type": "tencentcloud_instance", "name": "update", "change": {"actions": ["update"], "after": {"instance_type": "S5.LARGE8"}}},
        {"address": "tencentcloud_instance.delete", "type": "tencentcloud_instance", "name": "delete", "change": {"actions": ["delete"], "after": null}},
        {"address": "tencentcloud_instance.noop", "type": "tencentcloud_instance", "name": "noop", "change": {"actions": ["no-op"], "after": {"instance_type": "S5.SMALL2"}}}
      ]
    }`)

	plan, err := LoadPlanInventoryJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanInventoryJSON: %v", err)
	}
	if len(plan.Resources) != 1 || plan.Resources[0].Address != "tencentcloud_instance.update" {
		t.Fatalf("fallback inventory = %+v, want only update", plan.Resources)
	}
}

func regionsByAddress(resources []PlannedResource) map[string]string {
	regions := make(map[string]string, len(resources))
	for _, resource := range resources {
		regions[resource.Address] = resource.Region
	}
	return regions
}

func TestLoadPlanJSONResolvesExactProviderConfigRegions(t *testing.T) {
	path := writePlanJSON(t, `{
      "format_version": "1.2",
      "configuration": {
        "provider_config": {
          "aws": {"expressions": {"region": {"constant_value": "us-east-1"}}},
          "aws.west": {"expressions": {"region": {"constant_value": "us-west-2"}}}
        },
        "root_module": {
          "resources": [
            {"address": "aws_instance.default", "provider_config_key": "aws"},
            {"address": "aws_instance.west", "provider_config_key": "aws.west"},
            {"address": "aws_instance.explicit", "provider_config_key": "aws.west"}
          ]
        }
      },
      "resource_changes": [
        {"address": "aws_instance.default", "type": "aws_instance", "name": "default", "change": {"actions": ["create"], "after": {"instance_type": "m5.large"}}},
        {"address": "aws_instance.west", "type": "aws_instance", "name": "west", "change": {"actions": ["create"], "after": {"instance_type": "m5.large"}}},
        {"address": "aws_instance.explicit", "type": "aws_instance", "name": "explicit", "change": {"actions": ["create"], "after": {"instance_type": "m5.large", "region": "eu-central-1"}}}
      ]
    }`)

	plan, err := LoadPlanJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanJSON: %v", err)
	}
	got := regionsByAddress(plan.Resources)
	want := map[string]string{
		"aws_instance.default":  "us-east-1",
		"aws_instance.west":     "us-west-2",
		"aws_instance.explicit": "eu-central-1",
	}
	for address, region := range want {
		if got[address] != region {
			t.Errorf("region for %s = %q, want %q", address, got[address], region)
		}
	}
}

func TestLoadPlanInventoryJSONResolvesAliasesInExpandedNestedModules(t *testing.T) {
	path := writePlanJSON(t, `{
      "format_version": "1.2",
      "configuration": {
        "provider_config": {
          "aws": {"expressions": {"region": {"constant_value": "us-east-1"}}},
          "aws.west": {"expressions": {"region": {"constant_value": "us-west-2"}}},
          "aws.eu": {"expressions": {"region": {"constant_value": "eu-west-1"}}}
        },
        "root_module": {
          "module_calls": {
            "workers": {"module": {
              "resources": [
                {"address": "module.workers.aws_instance.node", "provider_config_key": "aws.eu"}
              ],
              "module_calls": {
                "nested": {"module": {
                  "resources": [
                    {"address": "module.workers.module.nested.aws_instance.node", "provider_config_key": "aws.west"}
                  ]
                }}
              }
            }}
          }
        }
      },
      "planned_values": {"root_module": {
        "child_modules": [{
          "address": "module.workers[0]",
          "resources": [
            {"address": "module.workers[0].aws_instance.node[0]", "mode": "managed", "type": "aws_instance", "name": "node", "values": {"instance_type": "m5.large"}}
          ],
          "child_modules": [{
            "address": "module.workers[0].module.nested[\"blue\"]",
            "resources": [
              {"address": "module.workers[0].module.nested[\"blue\"].aws_instance.node", "mode": "managed", "type": "aws_instance", "name": "node", "values": {"instance_type": "m5.large"}}
            ]
          }]
        }]
      }}
    }`)

	plan, err := LoadPlanInventoryJSON(path)
	if err != nil {
		t.Fatalf("LoadPlanInventoryJSON: %v", err)
	}
	got := regionsByAddress(plan.Resources)
	want := map[string]string{
		"module.workers[0].aws_instance.node[0]":                      "eu-west-1",
		"module.workers[0].module.nested[\"blue\"].aws_instance.node": "us-west-2",
	}
	for address, region := range want {
		if got[address] != region {
			t.Errorf("region for %s = %q, want %q", address, got[address], region)
		}
	}
}

func TestLoadPlanJSONLegacyProviderFallbackPrefersBaseDeterministically(t *testing.T) {
	path := writePlanJSON(t, `{
      "format_version": "1.2",
      "configuration": {"provider_config": {
        "aws.zed": {"expressions": {"region": {"constant_value": "us-west-1"}}},
        "aws": {"expressions": {"region": {"constant_value": "us-east-2"}}},
        "aws.alpha": {"expressions": {"region": {"constant_value": "eu-west-1"}}}
      }},
      "resource_changes": [
        {"address": "aws_instance.legacy", "type": "aws_instance", "name": "legacy", "change": {"actions": ["create"], "after": {"instance_type": "m5.large"}}}
      ]
    }`)

	for i := 0; i < 20; i++ {
		plan, err := LoadPlanJSON(path)
		if err != nil {
			t.Fatalf("LoadPlanJSON: %v", err)
		}
		if got := plan.Resources[0].Region; got != "us-east-2" {
			t.Fatalf("legacy fallback region = %q, want base provider us-east-2", got)
		}
	}
}
