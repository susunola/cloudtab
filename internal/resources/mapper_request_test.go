package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

// TestHuaweiMapperRequestBodies locks the request body each Huawei mapper
// produces (code review #1/#2). Before the fix, every mapper sent
// usage_factor="1" (EVS: "size") and project_id=region — both wrong. We assert
// here that usage_factor is "Duration", project_id is never set by a mapper, and
// EVS additionally carries resource_size + size_measure_id (17 = GB).
func TestHuaweiMapperRequestBodies(t *testing.T) {
	cases := []struct {
		name  string
		m     Mapper
		res   parser.PlannedResource
		isEVS bool
	}{
		{"ecs", HuaweiECS{}, parser.PlannedResource{Type: "huaweicloud_compute_instance", Region: "cn-north-4", After: map[string]interface{}{"flavor_id": "s3.large.2"}}, false},
		{"evs", HuaweiEVS{}, parser.PlannedResource{Type: "huaweicloud_evs_volume", Region: "cn-north-4", After: map[string]interface{}{"volume_type": "SAS", "size": 100}}, true},
		{"eip", HuaweiEIP{}, parser.PlannedResource{Type: "huaweicloud_vpc_eip", Region: "cn-north-4", After: map[string]interface{}{}}, false},
		{"elb", HuaweiELB{}, parser.PlannedResource{Type: "huaweicloud_elb_loadbalancer", Region: "cn-north-4", After: map[string]interface{}{}}, false},
		{"rds", HuaweiRDS{}, parser.PlannedResource{Type: "huaweicloud_rds_instance", Region: "cn-north-4", After: map[string]interface{}{}}, false},
		{"dcs", HuaweiDCS{}, parser.PlannedResource{Type: "huaweicloud_dcs_instance", Region: "cn-north-4", After: map[string]interface{}{}}, false},
		{"dds", HuaweiDDS{}, parser.PlannedResource{Type: "huaweicloud_dds_instance", Region: "cn-north-4", After: map[string]interface{}{}}, false},
		{"nat", HuaweiNAT{}, parser.PlannedResource{Type: "huaweicloud_nat_gateway", Region: "cn-north-4", After: map[string]interface{}{}}, false},
		{"cce", HuaweiCCE{}, parser.PlannedResource{Type: "huaweicloud_cce_cluster", Region: "cn-north-4", After: map[string]interface{}{}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.m.Extract(tc.res)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			// Bug #2 lock: mapper must never set project_id (region is not a project id).
			if _, ok := req.Params["project_id"]; ok {
				t.Errorf("mapper %s must not set project_id in Params", tc.name)
			}
			pis, ok := req.Params["product_infos"].([]map[string]interface{})
			if !ok || len(pis) == 0 {
				t.Fatalf("product_infos missing or empty")
			}
			pi := pis[0]
			// Critical bug #1 lock.
			if pi["usage_factor"] != "Duration" {
				t.Errorf("usage_factor = %v, want \"Duration\"", pi["usage_factor"])
			}
			if tc.isEVS {
				if pi["resource_size"] == nil {
					t.Errorf("EVS missing resource_size")
				}
				if pi["size_measure_id"] == nil {
					t.Errorf("EVS missing size_measure_id")
				}
			} else {
				if pi["resource_size"] != nil {
					t.Errorf("resource_size = %v, want nil for non-linear product", pi["resource_size"])
				}
				if pi["size_measure_id"] != nil {
					t.Errorf("size_measure_id = %v, want nil for non-linear product", pi["size_measure_id"])
				}
			}
		})
	}
}

// TestAlibabaMapperRequestBodies locks the request body each Alibaba mapper
// produces (code review #7/#8). Before the fix, mappers sent a dead "Quantity"
// field and built ModuleList entries by hand. We assert here that Quantity is
// gone and the ModuleList carries the expected primary ModuleCode.
func TestAlibabaMapperRequestBodies(t *testing.T) {
	cases := []struct {
		name  string
		m     Mapper
		res   parser.PlannedResource
		code  string
		primaryModule string
	}{
		{"ecs", AlibabaECS{}, parser.PlannedResource{Type: "alicloud_instance", Region: "cn-hangzhou", After: map[string]interface{}{"instance_type": "ecs.s6.large.2"}}, "ecs", "InstanceType"},
		{"disk", AlibabaDisk{}, parser.PlannedResource{Type: "alicloud_disk", Region: "cn-hangzhou", After: map[string]interface{}{"category": "cloud_essd", "size": 100}}, "disk", "DiskSize"},
		{"eip", AlibabaEIP{}, parser.PlannedResource{Type: "alicloud_eip", Region: "cn-hangzhou", After: map[string]interface{}{"bandwidth": 5}}, "eip", "Bandwidth"},
		{"slb", AlibabaSLB{}, parser.PlannedResource{Type: "alicloud_slb_load_balancer", Region: "cn-hangzhou", After: map[string]interface{}{}}, "slb", "Specification"},
		{"rds", AlibabaRDS{}, parser.PlannedResource{Type: "alicloud_db_instance", Region: "cn-hangzhou", After: map[string]interface{}{"instance_type": "rds.mysql.c2.large"}}, "rds", "DBInstanceClass"},
		{"redis", AlibabaRedis{}, parser.PlannedResource{Type: "alicloud_kvstore_instance", Region: "cn-hangzhou", After: map[string]interface{}{}}, "redisa", "InstanceClass"},
		{"mongodb", AlibabaMongoDB{}, parser.PlannedResource{Type: "alicloud_mongodb_instance", Region: "cn-hangzhou", After: map[string]interface{}{}}, "dds", "DBInstanceClass"},
		{"vpn", AlibabaVPN{}, parser.PlannedResource{Type: "alicloud_vpn_gateway", Region: "cn-hangzhou", After: map[string]interface{}{}}, "vpn", "Bandwidth"},
		{"nat", AlibabaNAT{}, parser.PlannedResource{Type: "alicloud_nat_gateway", Region: "cn-hangzhou", After: map[string]interface{}{}}, "natgateway", "Specification"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.m.Extract(tc.res)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			// Dead field lock (#7).
			if _, ok := req.Params["Quantity"]; ok {
				t.Errorf("mapper %s must not set Quantity in Params", tc.name)
			}
			ml, ok := req.Params["ModuleList"].([]map[string]string)
			if !ok || len(ml) == 0 {
				t.Fatalf("ModuleList missing or empty")
			}
			if ml[0]["ModuleCode"] != tc.primaryModule {
				t.Errorf("ModuleList[0].ModuleCode = %q, want %q", ml[0]["ModuleCode"], tc.primaryModule)
			}
			if ml[0]["PriceType"] != "Hour" {
				t.Errorf("ModuleList[0].PriceType = %q, want Hour", ml[0]["PriceType"])
			}
			if ml[0]["Config"] == "" {
				t.Errorf("ModuleList[0].Config is empty")
			}
		})
	}
}
