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

func TestPostgreSQLExtractAndParseFullPipeline(t *testing.T) {
	m := PostgreSQLInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_postgresql_instance.pg",
		Type:    "tencentcloud_postgresql_instance",
		Region:  "ap-beijing",
		After: map[string]interface{}{
			"availability_zone":    "ap-beijing-3",
			"spec_code":            "cdb.pg.z1.2g",
			"storage":              100,
			"instance_charge_type": "PREPAID",
			"prepaid_period":       1,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Postgres Extract: %v", err)
	}

	// InquiryPriceCreateDBInstances response.
	raw := []byte(`{"Response":{"Price":15000,"Currency":"CNY"}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Postgres Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if comps[0].MonthlyCost != 150.0 {
		t.Errorf("monthly = %v, want 150.0", comps[0].MonthlyCost)
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

// ----- VPN Gateway integration test -----

func TestVPNGatewayExtractAndParse(t *testing.T) {
	m := VPNGateway{}
	r := parser.PlannedResource{
		Address: "tencentcloud_vpn_gateway.gw",
		Type:    "tencentcloud_vpn_gateway",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"bandwidth":   10,
			"charge_type": "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("VPN Extract: %v", err)
	}
	if req.Product != "vpc" || req.Action != "InquiryPriceCreateVpnGateway" {
		t.Fatalf("VPN product/action = %q/%q", req.Product, req.Action)
	}

	// Response.Price.InstancePrice is in 元; UnitPrice is 元/h for POSTPAID.
	raw := []byte(`{"Response":{"Price":{"InstancePrice":{"UnitPrice":0.5,"ChargeUnit":"HOUR"}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("VPN Parse: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("VPN returned 0 components")
	}
	// POSTPAID hourly 0.5元 → monthly 0.5*730.
	if comps[0].HourlyCost != 0.5 {
		t.Errorf("VPN hourly = %v, want 0.5", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.5*hoursPerMonth {
		t.Errorf("VPN monthly = %v, want %v", comps[0].MonthlyCost, 0.5*hoursPerMonth)
	}
}

func TestVPNGatewayPrepaidWithBandwidth(t *testing.T) {
	m := VPNGateway{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_vpn_gateway",
		Region: "ap-shanghai",
		After: map[string]interface{}{
			"bandwidth":      20,
			"charge_type":    "PREPAID",
			"prepaid_period": 1,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("VPN Extract: %v", err)
	}
	if _, ok := req.Params["InstanceChargePrepaid"]; !ok {
		t.Error("PREPAID VPN should carry InstanceChargePrepaid")
	}
	// PREPAID monthly total + a separate bandwidth line.
	raw := []byte(`{"Response":{"Price":{"InstancePrice":{"DiscountPrice":100,"ChargeUnit":"MONTH"},"BandwidthPrice":{"UnitPrice":0.2,"ChargeUnit":"HOUR"}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("VPN Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("VPN components = %d, want 2 (instance+bandwidth)", len(comps))
	}
	if comps[0].MonthlyCost != 100 {
		t.Errorf("VPN instance monthly = %v, want 100", comps[0].MonthlyCost)
	}
}

// ----- MongoDB integration test -----

func TestMongoDBExtractAndParse(t *testing.T) {
	m := MongoDBInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_mongodb_instance.mongo",
		Type:    "tencentcloud_mongodb_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"memory":         4,
			"volume":         100,
			"charge_type":    "PREPAID",
			"prepaid_period": 1,
			"node_num":       3,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Mongo Extract: %v", err)
	}
	if req.Product != "mongodb" || req.Action != "InquirePriceCreateDBInstances" {
		t.Fatalf("Mongo product/action = %q/%q", req.Product, req.Action)
	}

	// PREPAID: DiscountPrice is a period (monthly) total in 元.
	raw := []byte(`{"Response":{"Price":{"UnitPrice":0,"OriginalPrice":300,"DiscountPrice":250}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Mongo Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("Mongo components = %d, want 1", len(comps))
	}
	if comps[0].MonthlyCost != 250 {
		t.Errorf("Mongo monthly = %v, want 250", comps[0].MonthlyCost)
	}
	if comps[0].HourlyCost != 0 {
		t.Errorf("Mongo PREPAID hourly = %v, want 0", comps[0].HourlyCost)
	}
}

func TestMongoDBPostpaidHourly(t *testing.T) {
	m := MongoDBInstance{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_mongodb_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"memory":         4,
			"volume":         100,
			"charge_type":    "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Mongo Extract: %v", err)
	}
	// POSTPAID: UnitPrice is 元/h.
	raw := []byte(`{"Response":{"Price":{"UnitPrice":1.2,"DiscountPrice":1.2}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Mongo Parse: %v", err)
	}
	if comps[0].HourlyCost != 1.2 {
		t.Errorf("Mongo hourly = %v, want 1.2", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 1.2*hoursPerMonth {
		t.Errorf("Mongo monthly = %v, want %v", comps[0].MonthlyCost, 1.2*hoursPerMonth)
	}
}

// ----- MariaDB integration test -----

func TestMariaDBExtractAndParse(t *testing.T) {
	m := MariaDBInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_mariadb_instance.maria",
		Type:    "tencentcloud_mariadb_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"zones":                []interface{}{"ap-guangzhou-3", "ap-guangzhou-4"},
			"memory":               8,
			"storage":              200,
			"node_count":           2,
			"instance_charge_type": "PREPAID",
			"period":               1,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Maria Extract: %v", err)
	}
	if req.Product != "mariadb" || req.Action != "DescribePrice" {
		t.Fatalf("Maria product/action = %q/%q", req.Product, req.Action)
	}
	if req.Params["Zone"] != "ap-guangzhou-3" {
		t.Errorf("Maria Zone = %v, want first of zones list", req.Params["Zone"])
	}

	// PREPAID DescribePrice returns 分 (period total); 15000分 = 150元.
	raw := []byte(`{"Response":{"Price":15000,"OriginalPrice":20000}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Maria Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("Maria components = %d, want 1", len(comps))
	}
	if comps[0].MonthlyCost != 150 {
		t.Errorf("Maria monthly = %v, want 150", comps[0].MonthlyCost)
	}
}

func TestMariaDBPostpaidHourly(t *testing.T) {
	m := MariaDBInstance{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_mariadb_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3",
			"memory":            8,
			"storage":           200,
			"charge_type":       "POSTPAID",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Maria Extract: %v", err)
	}
	// POSTPAID: 50分/h = 0.5元/h.
	raw := []byte(`{"Response":{"Price":50}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Maria Parse: %v", err)
	}
	if comps[0].HourlyCost != 0.5 {
		t.Errorf("Maria hourly = %v, want 0.5", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.5*hoursPerMonth {
		t.Errorf("Maria monthly = %v, want %v", comps[0].MonthlyCost, 0.5*hoursPerMonth)
	}
}

// ----- TDSQL-C (cynosdb) integration test -----

func TestCynosDBExtractAndParse(t *testing.T) {
	m := CynosDBCluster{}
	r := parser.PlannedResource{
		Address: "tencentcloud_cynosdb_cluster.tdsqlc",
		Type:    "tencentcloud_cynosdb_cluster",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"cpu":            2,
			"memory":         4,
			"storage_limit":  100,
			"charge_type":    "PREPAID",
			"prepaid_period": 1,
			"instance_count": 1,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Cynos Extract: %v", err)
	}
	if req.Product != "cynosdb" || req.Action != "InquirePriceCreate" {
		t.Fatalf("Cynos product/action = %q/%q", req.Product, req.Action)
	}

	// PREPAID: TotalPriceDiscount is 分. instance 20000分=200元 + storage 5000分=50元.
	raw := []byte(`{"Response":{"InstancePrice":{"TotalPrice":25000,"TotalPriceDiscount":20000},"StoragePrice":{"TotalPrice":6000,"TotalPriceDiscount":5000}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Cynos Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("Cynos components = %d, want 2 (compute+storage)", len(comps))
	}
	if comps[0].MonthlyCost != 200 {
		t.Errorf("Cynos compute monthly = %v, want 200", comps[0].MonthlyCost)
	}
	if comps[1].MonthlyCost != 50 {
		t.Errorf("Cynos storage monthly = %v, want 50", comps[1].MonthlyCost)
	}
}

func TestCynosDBPostpaidHourly(t *testing.T) {
	m := CynosDBCluster{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_cynosdb_cluster",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"cpu":            2,
			"memory":         4,
			"charge_type":    "POSTPAID",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Cynos Extract: %v", err)
	}
	// POSTPAID: UnitPriceDiscount is 分/h. 100分/h = 1元/h.
	raw := []byte(`{"Response":{"InstancePrice":{"UnitPrice":120,"UnitPriceDiscount":100}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Cynos Parse: %v", err)
	}
	if len(comps) < 1 {
		t.Fatal("Cynos returned 0 components")
	}
	if comps[0].HourlyCost != 1.0 {
		t.Errorf("Cynos hourly = %v, want 1.0", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 1.0*hoursPerMonth {
		t.Errorf("Cynos monthly = %v, want %v", comps[0].MonthlyCost, 1.0*hoursPerMonth)
	}
}

// TestPrepaidPricesSingleMonth guards against the period-total inflation bug:
// even when the user configures a multi-month prepaid term, the pricing request
// must ask for a single month so the returned price equals the monthly cost.
func TestPrepaidPricesSingleMonth(t *testing.T) {
	// MongoDB: Period must be forced to 1 regardless of prepaid_period.
	mongoReq, err := (MongoDBInstance{}).Extract(parser.PlannedResource{
		Type:   "tencentcloud_mongodb_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3", "memory": 4, "volume": 100,
			"charge_type": "PREPAID", "prepaid_period": 12,
		},
	})
	if err != nil {
		t.Fatalf("mongo extract: %v", err)
	}
	if got := mongoReq.Params["Period"]; got != 1 {
		t.Errorf("mongo Period = %v, want 1 (multi-month must not leak)", got)
	}

	// MariaDB: Period must be forced to 1.
	mariaReq, err := (MariaDBInstance{}).Extract(parser.PlannedResource{
		Type:   "tencentcloud_mariadb_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "memory": 8, "storage": 200,
			"charge_type": "PREPAID", "period": 36,
		},
	})
	if err != nil {
		t.Fatalf("maria extract: %v", err)
	}
	if got := mariaReq.Params["Period"]; got != 1 {
		t.Errorf("maria Period = %v, want 1", got)
	}

	// CynosDB: TimeSpan must be forced to 1 month for PREPAID.
	cynosReq, err := (CynosDBCluster{}).Extract(parser.PlannedResource{
		Type:   "tencentcloud_cynosdb_cluster",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3", "cpu": 2, "memory": 4,
			"charge_type": "PREPAID", "prepaid_period": 24,
		},
	})
	if err != nil {
		t.Fatalf("cynos extract: %v", err)
	}
	if got := cynosReq.Params["TimeSpan"]; got != 1 {
		t.Errorf("cynos TimeSpan = %v, want 1", got)
	}
	if got := cynosReq.Params["TimeUnit"]; got != "m" {
		t.Errorf("cynos TimeUnit = %v, want m", got)
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
		"tencentcloud_postgresql_instance",
		"tencentcloud_redis_instance",
		"tencentcloud_vpn_gateway",
		"tencentcloud_mongodb_instance",
		"tencentcloud_mariadb_instance",
		"tencentcloud_cynosdb_cluster",
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
		{
			addr: "tencentcloud_postgresql_instance.pg",
			typ:  "tencentcloud_postgresql_instance",
			after: map[string]interface{}{
				"availability_zone":    "ap-guangzhou-3",
				"spec_code":            "cdb.pg.z1.2g",
				"storage":              150,
				"instance_charge_type": "POSTPAID_BY_HOUR",
			},
		},
		{
			addr: "tencentcloud_vpn_gateway.gw",
			typ:  "tencentcloud_vpn_gateway",
			after: map[string]interface{}{
				"bandwidth":   10,
				"charge_type": "POSTPAID_BY_HOUR",
			},
		},
		{
			addr: "tencentcloud_mongodb_instance.mongo",
			typ:  "tencentcloud_mongodb_instance",
			after: map[string]interface{}{
				"available_zone": "ap-guangzhou-3",
				"memory":         4,
				"volume":         100,
				"charge_type":    "POSTPAID_BY_HOUR",
			},
		},
		{
			addr: "tencentcloud_mariadb_instance.maria",
			typ:  "tencentcloud_mariadb_instance",
			after: map[string]interface{}{
				"availability_zone": "ap-guangzhou-3",
				"memory":            8,
				"storage":           200,
				"charge_type":       "POSTPAID",
			},
		},
		{
			addr: "tencentcloud_cynosdb_cluster.tdsqlc",
			typ:  "tencentcloud_cynosdb_cluster",
			after: map[string]interface{}{
				"available_zone": "ap-guangzhou-3",
				"cpu":            2,
				"memory":         4,
				"charge_type":    "POSTPAID",
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
