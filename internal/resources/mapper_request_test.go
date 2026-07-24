package resources

import (
	"strings"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

// TestHuaweiMapperRequestBodies locks the request body each Huawei mapper
// produces (code review #1/#2, plus the EIP/upflow validation item).
//
// Before the fix, every mapper sent usage_factor="1" (EVS: "size") and
// project_id=region — both wrong. We now assert:
//   - usage_factor is "Duration" (or "upflow" for EIP billed by traffic),
//   - usage_measure_id is 4 (hour) for Duration, 10 (GB) for upflow,
//   - project_id is never set by a mapper,
//   - EVS carries resource_size + size_measure_id (17 = GB),
//   - EIP models the public IP (resource_type "hws.resource.type.ip") AND the
//     linear bandwidth (resource_type "hws.resource.type.bandwidth",
//     size_measure_id 15 = Mbps); the bandwidth entry is "upflow" only when the
//     plan's charge_mode is "traffic".
func TestHuaweiMapperRequestBodies(t *testing.T) {
	cases := []struct {
		name      string
		m         Mapper
		res       parser.PlannedResource
		isEVS     bool
		isEIP     bool
		byTraffic bool
	}{
		{"ecs", HuaweiECS{}, parser.PlannedResource{Type: "huaweicloud_compute_instance", Region: "cn-north-4", After: map[string]interface{}{"flavor_id": "s3.large.2"}}, false, false, false},
		{"evs", HuaweiEVS{}, parser.PlannedResource{Type: "huaweicloud_evs_volume", Region: "cn-north-4", After: map[string]interface{}{"volume_type": "SAS", "size": 100}}, true, false, false},
		{"eip", HuaweiEIP{}, parser.PlannedResource{Type: "huaweicloud_vpc_eip", Region: "cn-north-4", After: map[string]interface{}{}}, false, true, false},
		{"eip-traffic", HuaweiEIP{}, parser.PlannedResource{Type: "huaweicloud_vpc_eip", Region: "cn-north-4", After: map[string]interface{}{"bandwidth": map[string]interface{}{"charge_mode": "traffic", "size": 10}}}, false, true, true},
		{"elb", HuaweiELB{}, parser.PlannedResource{Type: "huaweicloud_elb_loadbalancer", Region: "cn-north-4", After: map[string]interface{}{}}, false, false, false},
		{"rds", HuaweiRDS{}, parser.PlannedResource{Type: "huaweicloud_rds_instance", Region: "cn-north-4", After: map[string]interface{}{}}, false, false, false},
		{"dcs", HuaweiDCS{}, parser.PlannedResource{Type: "huaweicloud_dcs_instance", Region: "cn-north-4", After: map[string]interface{}{}}, false, false, false},
		{"dds", HuaweiDDS{}, parser.PlannedResource{Type: "huaweicloud_dds_instance", Region: "cn-north-4", After: map[string]interface{}{}}, false, false, false},
		{"nat", HuaweiNAT{}, parser.PlannedResource{Type: "huaweicloud_nat_gateway", Region: "cn-north-4", After: map[string]interface{}{}}, false, false, false},
		{"cce", HuaweiCCE{}, parser.PlannedResource{Type: "huaweicloud_cce_cluster", Region: "cn-north-4", After: map[string]interface{}{}}, false, false, false},
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

			// Consistency: if resource_size is present, size_measure_id must be too (and vice versa).
			checkConsistency := func(pi map[string]interface{}) {
				hasSize := pi["resource_size"] != nil
				hasMeasure := pi["size_measure_id"] != nil
				if hasSize != hasMeasure {
					t.Errorf("resource_size/size_measure_id inconsistency: size=%v measure=%v", hasSize, hasMeasure)
				}
			}

			if tc.isEVS {
				if len(pis) != 1 {
					t.Fatalf("EVS should have 1 product_info, got %d", len(pis))
				}
				pi := pis[0]
				checkConsistency(pi)
				if pi["resource_size"] == nil || pi["size_measure_id"] == nil {
					t.Errorf("EVS must carry resource_size + size_measure_id")
				}
				if pi["size_measure_id"] != int32(17) {
					t.Errorf("EVS size_measure_id = %v, want 17 (GB)", pi["size_measure_id"])
				}
				if pi["usage_factor"] != "Duration" {
					t.Errorf("EVS usage_factor = %v, want Duration", pi["usage_factor"])
				}
				if pi["usage_measure_id"] != int32(4) {
					t.Errorf("EVS usage_measure_id = %v, want 4 (hour)", pi["usage_measure_id"])
				}
			} else if tc.isEIP {
				if len(pis) != 2 {
					t.Fatalf("EIP should have 2 product_infos (ip + bandwidth), got %d", len(pis))
				}
				var ipEntry, bwEntry map[string]interface{}
				for _, pi := range pis {
					rt, _ := pi["resource_type"].(string)
					switch rt {
					case "hws.resource.type.ip":
						ipEntry = pi
					case "hws.resource.type.bandwidth":
						bwEntry = pi
					default:
						t.Errorf("EIP unexpected resource_type %q", rt)
					}
				}
				if ipEntry == nil {
					t.Errorf("EIP missing public-IP product_info (hws.resource.type.ip)")
				}
				if bwEntry == nil {
					t.Fatalf("EIP missing bandwidth product_info (hws.resource.type.bandwidth)")
				}
				checkConsistency(ipEntry)
				checkConsistency(bwEntry)
				// IP entry: Duration, no resource_size.
				if ipEntry["usage_factor"] != "Duration" || ipEntry["usage_measure_id"] != int32(4) {
					t.Errorf("EIP IP entry usage_factor/measure = %v/%v, want Duration/4", ipEntry["usage_factor"], ipEntry["usage_measure_id"])
				}
				if ipEntry["resource_size"] != nil {
					t.Errorf("EIP IP entry must not carry resource_size")
				}
				// Bandwidth entry: linear (resource_size + size_measure_id 15), by-bandwidth (Duration/4) or by-traffic (upflow/10).
				if bwEntry["resource_size"] == nil || bwEntry["size_measure_id"] != int32(15) {
					t.Errorf("EIP bandwidth entry must carry resource_size + size_measure_id 15 (Mbps), got size=%v measure=%v", bwEntry["resource_size"], bwEntry["size_measure_id"])
				}
				wantUF := "Duration"
				wantMeasure := int32(4)
				if tc.byTraffic {
					wantUF = "upflow"
					wantMeasure = 10
				}
				if bwEntry["usage_factor"] != wantUF || bwEntry["usage_measure_id"] != wantMeasure {
					t.Errorf("EIP bandwidth usage_factor/measure = %v/%v, want %s/%d", bwEntry["usage_factor"], bwEntry["usage_measure_id"], wantUF, wantMeasure)
				}
			} else {
				if len(pis) != 1 {
					t.Fatalf("%s should have 1 product_info, got %d", tc.name, len(pis))
				}
				pi := pis[0]
				checkConsistency(pi)
				if pi["usage_factor"] != "Duration" {
					t.Errorf("usage_factor = %v, want \"Duration\"", pi["usage_factor"])
				}
				if pi["usage_measure_id"] != int32(4) {
					t.Errorf("usage_measure_id = %v, want 4 (hour)", pi["usage_measure_id"])
				}
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
// produces (code review #7/#8, plus the product-code / Config-format validation
// item). We assert: Product code is correct (disk->yundisk, nat->nat_gw), no dead
// Quantity field, the primary ModuleCode is present, and each Config uses the
// documented "PropertyCode:Value" format (config must start with "ModuleCode:").
func TestAlibabaMapperRequestBodies(t *testing.T) {
	cases := []struct {
		name          string
		m             Mapper
		res           parser.PlannedResource
		code          string
		primaryModule string
		priceType     string
	}{
		{"ecs", AlibabaECS{}, parser.PlannedResource{Type: "alicloud_instance", Region: "cn-hangzhou", After: map[string]interface{}{"instance_type": "ecs.s6.large.2"}}, "ecs", "InstanceType", "Hour"},
		{"disk", AlibabaDisk{}, parser.PlannedResource{Type: "alicloud_disk", Region: "cn-hangzhou", After: map[string]interface{}{"category": "cloud_essd", "size": 100}}, "yundisk", "DataDisk", "Hour"},
		{"eip", AlibabaEIP{}, parser.PlannedResource{Type: "alicloud_eip", Region: "cn-hangzhou", After: map[string]interface{}{"bandwidth": 5}}, "eip", "Bandwidth", "Day"},
		{"slb", AlibabaSLB{}, parser.PlannedResource{Type: "alicloud_slb_load_balancer", Region: "cn-hangzhou", After: map[string]interface{}{}}, "slb", "LoadBalancerSpec", "Hour"},
		{"rds", AlibabaRDS{}, parser.PlannedResource{Type: "alicloud_db_instance", Region: "cn-hangzhou", After: map[string]interface{}{"instance_type": "rds.mysql.c2.large"}}, "rds", "DBInstanceClass", "Hour"},
		{"redis", AlibabaRedis{}, parser.PlannedResource{Type: "alicloud_kvstore_instance", Region: "cn-hangzhou", After: map[string]interface{}{}}, "redisa", "InstanceClass", "Hour"},
		{"mongodb", AlibabaMongoDB{}, parser.PlannedResource{Type: "alicloud_mongodb_instance", Region: "cn-hangzhou", After: map[string]interface{}{}}, "dds", "DBInstanceClass", "Hour"},
		{"vpn", AlibabaVPN{}, parser.PlannedResource{Type: "alicloud_vpn_gateway", Region: "cn-hangzhou", After: map[string]interface{}{"bandwidth": 10}}, "vpn", "Bandwidth", "Hour"},
		{"nat", AlibabaNAT{}, parser.PlannedResource{Type: "alicloud_nat_gateway", Region: "cn-hangzhou", After: map[string]interface{}{}}, "nat_gw", "Spec", "Day"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.m.Extract(tc.res)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			// Product code lock (disk->yundisk, nat->nat_gw).
			if req.Product != tc.code {
				t.Errorf("Product = %q, want %q", req.Product, tc.code)
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
			if ml[0]["PriceType"] != tc.priceType {
				t.Errorf("ModuleList[0].PriceType = %q, want %q", ml[0]["PriceType"], tc.priceType)
			}
			if ml[0]["Config"] == "" {
				t.Errorf("ModuleList[0].Config is empty")
			}
			// Documented "PropertyCode:Value" format lock: Config must begin with
			// the module code followed by ':' (e.g. "InstanceType:...") or '.' (e.g.
			// disk "DataDisk.Size:..."). Both are valid PropertyCode:Value shapes.
			cm := ml[0]["Config"]
			if !strings.HasPrefix(cm, tc.primaryModule) {
				t.Errorf("ModuleList[0].Config = %q, want it to start with %q (PropertyCode:Value format)", cm, tc.primaryModule)
			} else if next := cm[len(tc.primaryModule)]; next != ':' && next != '.' {
				t.Errorf("ModuleList[0].Config = %q, want %q followed by ':' or '.' separator", cm, tc.primaryModule)
			}
		})
	}
}
