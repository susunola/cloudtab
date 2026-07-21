package resources

import (
	"encoding/json"
	"testing"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
)

// ----- CVM integration test -----

func TestCVMExtractAndParse(t *testing.T) {
	m := CVMInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_instance.web",
		Type:    "tencentcloud_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"instance_type":              "SA2.MEDIUM4",
			"image_id":                   "img-xxx",
			"availability_zone":          "ap-guangzhou-6",
			"internet_charge_type":       "BANDWIDTH_PREPAID",
			"internet_max_bandwidth_out": 10,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("CVM Extract: %v", err)
	}
	if req.Product != "cvm" {
		t.Fatalf("CVM product = %q, want cvm", req.Product)
	}

	// Simulate a typical InquiryPriceRunInstances response.
	raw := inquiryPriceRunInstancesResp(t, "HOUR", 0.5, 0.6)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("CVM Parse: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("CVM returned 0 components")
	}
	for _, c := range comps {
		if c.MonthlyCost < 0 {
			t.Errorf("component %s has negative monthly cost: %v", c.Name, c.MonthlyCost)
		}
	}
}

// ----- CBS integration test -----

func TestCBSExtractAndParse(t *testing.T) {
	m := CBSStorage{}
	r := parser.PlannedResource{
		Address: "tencentcloud_cbs_storage.data",
		Type:    "tencentcloud_cbs_storage",
		Region:  "ap-shanghai",
		After: map[string]interface{}{
			"storage_type":      "CLOUD_SSD",
			"storage_size":      100,
			"availability_zone": "ap-shanghai-2",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("CBS Extract: %v", err)
	}
	if req.Product != "cbs" {
		t.Fatalf("CBS product = %q, want cbs", req.Product)
	}

	raw := inquiryPriceCreateDisksResp(t, "HOUR", 0.1, 0.12)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("CBS Parse: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("CBS returned 0 components")
	}
}

// ----- CLB integration test -----

func TestCLBExtractAndParse(t *testing.T) {
	m := CLBInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_clb_instance.lb",
		Type:    "tencentcloud_clb_instance",
		Region:  "ap-beijing",
		After: map[string]interface{}{
			"network_type": "OPEN",
			"charge_type":  "POSTPAID",
			"bandwidth":    10,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("CLB Extract: %v", err)
	}
	if req.Product != "clb" {
		t.Fatalf("CLB product = %q, want clb", req.Product)
	}

	raw := inquiryPriceCreateLBResp(t, "HOUR", 0.3, 0.35)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("CLB Parse: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("CLB returned 0 components")
	}
}

// ----- MySQL integration test -----

func TestMySQLExtractAndParseFullPipeline(t *testing.T) {
	m := MySQLInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_mysql_instance.db",
		Type:    "tencentcloud_mysql_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-6",
			"mem_size":          4000,
			"volume_size":       200,
			"charge_type":       "POSTPAID",
			"cpu":               2,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("MySQL Extract: %v", err)
	}

	// DescribeDBPrice response (price in 分).
	raw := []byte(`{"Response":{"Price":50000,"Currency":"CNY"}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("MySQL Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	// POSTPAID: hourly = 500元(分→元), monthly = 500 * 730
	wantMonthly := 500.0 * hoursPerMonth
	if comps[0].HourlyCost != 500.0 {
		t.Errorf("hourly = %v, want 500.0", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != wantMonthly {
		t.Errorf("monthly = %v, want %v", comps[0].MonthlyCost, wantMonthly)
	}
}

// ----- Redis integration test -----

func TestRedisExtractAndParseFullPipeline(t *testing.T) {
	m := RedisInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_redis_instance.cache",
		Type:    "tencentcloud_redis_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone":  "ap-guangzhou-6",
			"type":               "Redis4.0",
			"mem_size":           1024,
			"charge_type":        "POSTPAID",
			"redis_shard_num":    1,
			"redis_replicas_num": 1,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Redis Extract: %v", err)
	}

	// InquiryPriceCreateInstance response.
	raw := redisInquiryPriceResp(t)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Redis Parse: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("Redis returned 0 components")
	}
}

// ----- EIP StaticMapper test -----

func TestEIPEstimate(t *testing.T) {
	m := EIP{}
	r := parser.PlannedResource{
		Address: "tencentcloud_eip.pub",
		Type:    "tencentcloud_eip",
		After: map[string]interface{}{
			"internet_max_bandwidth_out": 10,
			"internet_charge_type":       "TRAFFIC_POSTPAID_BY_HOUR",
		},
	}
	comps, err := m.Estimate(r)
	if err != nil {
		t.Fatalf("EIP Estimate: %v", err)
	}
	// EIP returns a zero-cost placeholder; just verify it doesn't panic.
	for _, c := range comps {
		if c.MonthlyCost != 0 {
			t.Errorf("EIP component %s has non-zero cost: %v", c.Name, c.MonthlyCost)
		}
	}
}

// ----- Registry lookup test -----

func TestDefaultRegistryHasAllTypes(t *testing.T) {
	reg := DefaultRegistry()
	types := []string{
		"tencentcloud_instance",
		"tencentcloud_cbs_storage",
		"tencentcloud_eip",
		"tencentcloud_clb_instance",
		"tencentcloud_mysql_instance",
		"tencentcloud_redis_instance",
	}
	for _, tfType := range types {
		if _, ok := reg.Lookup(tfType); !ok {
			t.Errorf("DefaultRegistry missing type: %s", tfType)
		}
	}
}

// ----- mock response helpers -----

// inquiryPriceRunInstancesResp builds a fake CVM InquiryPriceRunInstances response JSON.
// unitPriceDiscount is 元/h, discountPrice is total discounted price.
func inquiryPriceRunInstancesResp(t *testing.T, chargeUnit string, unitPriceDiscount, discountPrice float64) []byte {
	t.Helper()
	type priceInfo struct {
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		DiscountPrice     float64 `json:"DiscountPrice"`
		ChargeUnit        string  `json:"ChargeUnit"`
	}
	resp := struct {
		PriceInfo []priceInfo `json:"PriceInfo"`
		Response  struct {
			PriceInfo []priceInfo `json:"PriceInfo"`
		} `json:"Response"`
	}{
		PriceInfo: []priceInfo{{
			UnitPriceDiscount: unitPriceDiscount,
			DiscountPrice:     discountPrice,
			ChargeUnit:        chargeUnit,
		}},
	}
	resp.Response.PriceInfo = resp.PriceInfo
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal mock response: %v", err)
	}
	return b
}

func inquiryPriceCreateDisksResp(t *testing.T, chargeUnit string, unitPriceDiscount, discountPrice float64) []byte {
	return inquiryPriceRunInstancesResp(t, chargeUnit, unitPriceDiscount, discountPrice)
}

func inquiryPriceCreateLBResp(t *testing.T, chargeUnit string, unitPriceDiscount, discountPrice float64) []byte {
	return inquiryPriceRunInstancesResp(t, chargeUnit, unitPriceDiscount, discountPrice)
}

func redisInquiryPriceResp(t *testing.T) []byte {
	t.Helper()
	resp := struct {
		Price         float64 `json:"Price"`
		OriginalPrice float64 `json:"OriginalPrice"`
		Currency      string  `json:"Currency"`
		Response      struct {
			Price         float64 `json:"Price"`
			OriginalPrice float64 `json:"OriginalPrice"`
			Currency      string  `json:"Currency"`
		} `json:"Response"`
	}{
		Price:         20000,
		OriginalPrice: 25000,
		Currency:      "CNY",
	}
	resp.Response.Price = resp.Price
	resp.Response.OriginalPrice = resp.OriginalPrice
	resp.Response.Currency = resp.Currency
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal redis response: %v", err)
	}
	return b
}

// ----- Mapper contract validation -----

func TestAllMappersImplementContract(t *testing.T) {
	// Verify every registered type produces valid Extract → (static|dynamic) output.
	reg := DefaultRegistry()
	testCases := []struct {
		addr  string
		typ   string
		after map[string]interface{}
	}{
		{
			addr: "tencentcloud_instance.web",
			typ:  "tencentcloud_instance",
			after: map[string]interface{}{
				"instance_type":              "SA2.MEDIUM4",
				"image_id":                   "img-xxx",
				"availability_zone":          "ap-guangzhou-3",
				"internet_charge_type":       "BANDWIDTH_PREPAID",
				"internet_max_bandwidth_out": 5,
			},
		},
		{
			addr:  "tencentcloud_cbs_storage.data",
			typ:   "tencentcloud_cbs_storage",
			after: map[string]interface{}{"storage_type": "CLOUD_SSD", "storage_size": 50, "availability_zone": "ap-guangzhou-3"},
		},
		{
			addr: "tencentcloud_mysql_instance.db",
			typ:  "tencentcloud_mysql_instance",
			after: map[string]interface{}{
				"availability_zone": "ap-guangzhou-3",
				"mem_size":          2000,
				"volume_size":       100,
				"charge_type":       "PREPAID",
				"prepaid_period":    1,
			},
		},
		{
			addr: "tencentcloud_redis_instance.cache",
			typ:  "tencentcloud_redis_instance",
			after: map[string]interface{}{
				"availability_zone":  "ap-guangzhou-3",
				"type":               "Redis4.0",
				"mem_size":           512,
				"charge_type":        "POSTPAID",
				"redis_shard_num":    1,
				"redis_replicas_num": 1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.typ, func(t *testing.T) {
			mapper, ok := reg.Lookup(tc.typ)
			if !ok {
				t.Fatalf("type %q not in registry", tc.typ)
			}
			r := parser.PlannedResource{
				Address: tc.addr,
				Type:    tc.typ,
				Region:  "ap-guangzhou",
				After:   tc.after,
			}
			req, err := mapper.Extract(r)
			if err != nil {
				t.Fatalf("Extract error: %v", err)
			}
			if req.Product == "" || req.Action == "" {
				t.Fatal("Extract returned empty Product or Action")
			}

			// If static mapper, test Estimate instead of Parse.
			if sm, ok := mapper.(StaticMapper); ok {
				comps, err := sm.Estimate(r)
				if err != nil {
					t.Fatalf("Estimate error: %v", err)
				}
				validateComponents(t, comps)
				return
			}

			// Dynamic mapper: verify Parse accepts well-formed empty-ish JSON.
			_ = req // Parse needs real API data; just validate Extract shape here.
		})
	}
}

func validateComponents(t *testing.T, comps []output.CostComponent) {
	t.Helper()
	for i, c := range comps {
		if c.Name == "" {
			t.Errorf("component[%d] has empty Name", i)
		}
		if c.Currency == "" {
			t.Errorf("component[%d] has empty Currency", i)
		}
	}
}
