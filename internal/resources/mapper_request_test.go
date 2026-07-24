package resources

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

// assertParamsSubset recursively checks that every key in want exists in got
// with an equal value. Nested maps match recursively so the test can pin a
// nested field (e.g. ChargePrepaid.Period) without snapshotting the whole
// request body. Leaf values are compared after JSON normalization so numeric
// types that differ only in Go type (int vs int64) compare equal.
func assertParamsSubset(t *testing.T, path string, got, want map[string]interface{}) {
	t.Helper()
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("%s: missing param %q", path, k)
			continue
		}
		if wm, ok := wv.(map[string]interface{}); ok {
			gm, ok := gv.(map[string]interface{})
			if !ok {
				t.Errorf("%s: param %q = %v, want a nested map", path, k, gv)
				continue
			}
			assertParamsSubset(t, path+"."+k, gm, wm)
			continue
		}
		wj, _ := json.Marshal(wv)
		gj, _ := json.Marshal(gv)
		if string(wj) != string(gj) {
			t.Errorf("%s: param %q = %v, want %v", path, k, gv, wv)
		}
	}
}

// TestTencentMapperRequestBodies is the Tencent counterpart to
// TestHuaweiMapperRequestBodies / TestAlibabaMapperRequestBodies: it locks the
// request each Tencent mapper sends to its pricing API. This is the gap that
// let the PREPAID period-total bug hide in six mappers — the price is decided
// by the request, and no test was looking at it. For every mapper we assert the
// exact Product/Action and the price-determining params, and for every
// prepaid-capable mapper that the period is forced to a single month
// (Period=1 / TimeSpan=1) so a multi-month term can never be reported as one
// month again.
func TestTencentMapperRequestBodies(t *testing.T) {
	cases := []struct {
		name    string
		m       Mapper
		res     parser.PlannedResource
		product string
		action  string
		want    map[string]interface{}
		static  bool // StaticMapper: Extract must refuse (no request body)
	}{
		{"cvm", CVMInstance{}, parser.PlannedResource{Type: "tencentcloud_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"instance_type": "S5.LARGE8", "image_id": "img-xxx", "availability_zone": "ap-guangzhou-6",
			"instance_charge_type": "PREPAID", "instance_charge_type_prepaid_period": 12}},
			"cvm", "InquiryPriceRunInstances", map[string]interface{}{
				"InstanceChargeType": "PREPAID", "InstanceChargePrepaid": map[string]interface{}{"Period": 1}}, false},
		{"cbs", CBSStorage{}, parser.PlannedResource{Type: "tencentcloud_cbs_storage", Region: "ap-guangzhou", After: map[string]interface{}{
			"storage_type": "CLOUD_PREMIUM", "storage_size": 100, "availability_zone": "ap-guangzhou-6",
			"charge_type": "PREPAID", "prepaid_period": 12}},
			"cbs", "InquiryPriceCreateDisks", map[string]interface{}{
				"DiskChargeType": "PREPAID", "DiskChargePrepaid": map[string]interface{}{"Period": 1}}, false},
		{"eip", EIP{}, parser.PlannedResource{Type: "tencentcloud_eip", Region: "ap-guangzhou", After: map[string]interface{}{}},
			"", "", nil, true},
		{"clb", CLBInstance{}, parser.PlannedResource{Type: "tencentcloud_clb_instance", Region: "ap-guangzhou", After: map[string]interface{}{}},
			"clb", "InquiryPriceCreateLoadBalancer", map[string]interface{}{
				"LoadBalancerChargeType": "POSTPAID", "GoodsNum": 1}, false},
		{"mysql", MySQLInstance{}, parser.PlannedResource{Type: "tencentcloud_mysql_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-6", "mem_size": 4000, "volume_size": 200,
			"charge_type": "PREPAID", "prepaid_period": 12}},
			"cdb", "DescribeDBPrice", map[string]interface{}{"PayType": "PRE_PAID", "Period": 1}, false},
		{"postgresql", PostgreSQLInstance{}, parser.PlannedResource{Type: "tencentcloud_postgresql_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "spec_code": "pg.it2.large", "storage": 100,
			"instance_charge_type": "PREPAID", "prepaid_period": 12}},
			"postgres", "InquiryPriceCreateDBInstances", map[string]interface{}{
				"InstanceChargeType": "PREPAID", "Period": 1}, false},
		{"redis", RedisInstance{}, parser.PlannedResource{Type: "tencentcloud_redis_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "mem_size": 1024,
			"charge_type": "PREPAID", "prepaid_period": 12}},
			"redis", "InquiryPriceCreateInstance", map[string]interface{}{"BillingMode": 1, "Period": 1}, false},
		{"vpn", VPNGateway{}, parser.PlannedResource{Type: "tencentcloud_vpn_gateway", Region: "ap-guangzhou", After: map[string]interface{}{
			"bandwidth": 100, "charge_type": "PREPAID", "prepaid_period": 12}},
			"vpc", "InquiryPriceCreateVpnGateway", map[string]interface{}{
				"InstanceChargeType": "PREPAID", "InstanceChargePrepaid": map[string]interface{}{"Period": 1}}, false},
		{"mongodb", MongoDBInstance{}, parser.PlannedResource{Type: "tencentcloud_mongodb_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3", "memory": 4, "volume": 100,
			"charge_type": "PREPAID", "prepaid_period": 12}},
			"mongodb", "InquirePriceCreateDBInstances", map[string]interface{}{
				"Period": 1, "ClusterType": "REPLSET", "ReplicateSetNum": 1}, false},
		{"mariadb", MariaDBInstance{}, parser.PlannedResource{Type: "tencentcloud_mariadb_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "memory": 8, "storage": 200,
			"instance_charge_type": "prepaid", "period": 36}},
			"mariadb", "DescribePrice", map[string]interface{}{
				"Paymode": "prepaid", "Period": 1, "AmountUnit": "pent"}, false},
		{"cynosdb", CynosDBCluster{}, parser.PlannedResource{Type: "tencentcloud_cynosdb_cluster", Region: "ap-guangzhou", After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3", "cpu": 2, "memory": 4,
			"charge_type": "PREPAID", "prepaid_period": 24}},
			"cynosdb", "InquirePriceCreate", map[string]interface{}{
				"InstancePayMode": "PREPAID", "TimeSpan": 1, "TimeUnit": "m"}, false},
		{"lighthouse", LighthouseInstance{}, parser.PlannedResource{Type: "tencentcloud_lighthouse_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"bundle_id": "bundle_xxx", "instance_count": 1}},
			"lighthouse", "InquirePriceCreateInstances", map[string]interface{}{
				"InstanceChargePrepaid": map[string]interface{}{"Period": 1}}, false},
		{"ecm", ECMInstance{}, parser.PlannedResource{Type: "tencentcloud_ecm_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"instance_type": "S4.MEDIUM4"}},
			"ecm", "DescribePriceRunInstance", map[string]interface{}{"InstanceChargeType": 1}, false},
		{"sqlserver", SQLServerInstance{}, parser.PlannedResource{Type: "tencentcloud_sqlserver_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "memory": 8, "storage": 200,
			"charge_type": "PREPAID", "prepaid_period": 12}},
			"sqlserver", "InquiryPriceCreateDBInstances", map[string]interface{}{
				"InstanceChargeType": "PREPAID", "Period": 1}, false},
		{"dcdb", DCDBInstance{}, parser.PlannedResource{Type: "tencentcloud_dcdb_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "shard_memory": 8, "shard_storage": 200, "shard_count": 2,
			"instance_charge_type": "PREPAID"}},
			"dcdb", "DescribeDCDBPrice", map[string]interface{}{
				"Paymode": "prepaid", "Period": 1, "AmountUnit": "pent"}, false},
		{"gaap", GAAPProxy{}, parser.PlannedResource{Type: "tencentcloud_gaap_proxy", Region: "ap-guangzhou", After: map[string]interface{}{
			"access_region": "ap-guangzhou", "realserver_region": "ap-seoul", "bandwidth": 10, "concurrent": 2}},
			"gaap", "InquiryPriceCreateProxy", map[string]interface{}{"BillingType": 0}, false},
		{"yunjing", YunjingLicense{}, parser.PlannedResource{Type: "tencentcloud_cwp_license_order", Region: "ap-guangzhou", After: map[string]interface{}{}},
			"yunjing", "InquiryPriceOpenProVersionPrepaid", map[string]interface{}{
				"ChargePrepaid": map[string]interface{}{"Period": 1}}, false},
		{"cloudhsm", CloudHSMInstance{}, parser.PlannedResource{Type: "tencentcloud_cloudhsm_instance", Region: "ap-guangzhou", After: map[string]interface{}{}},
			"cloudhsm", "InquiryPriceBuyVsm", map[string]interface{}{
				"PayMode": 1, "TimeSpan": "1", "TimeUnit": "m"}, false},
		{"domain", DomainRegistration{}, parser.PlannedResource{Type: "tencentcloud_domain_registration", Region: "ap-guangzhou", After: map[string]interface{}{
			"domain_name": "example.com", "period": 1}},
			"domain", "DescribeDomainPriceList", map[string]interface{}{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.m.Extract(tc.res)
			if tc.static {
				if err == nil {
					t.Fatalf("static mapper %s should refuse Extract (use Estimate)", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			if req.Product != tc.product {
				t.Errorf("Product = %q, want %q", req.Product, tc.product)
			}
			if req.Action != tc.action {
				t.Errorf("Action = %q, want %q", req.Action, tc.action)
			}
			assertParamsSubset(t, tc.name, req.Params, tc.want)
		})
	}
}

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
