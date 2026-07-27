package resources

import (
	"encoding/json"
	"testing"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
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
	// POSTPAID hourly 0.5元 → monthly 0.5×730.
	if comps[0].HourlyCost != 0.5 {
		t.Errorf("CVM hourly = %v, want 0.5", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.5*hoursPerMonth {
		t.Errorf("CVM monthly = %v, want %v", comps[0].MonthlyCost, 0.5*hoursPerMonth)
	}
}

// TestCVMBandwidthUnitPriceFallback verifies that a CVM response which carries
// BandwidthPrice.UnitPrice but leaves UnitPriceDiscount at 0 still produces a
// non-zero bandwidth cost. Previously monthlyFromPrice received 0/0 and emitted
// a zero-cost line.
func TestCVMBandwidthUnitPriceFallback(t *testing.T) {
	m := CVMInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_instance.web",
		Type:    "tencentcloud_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"instance_type":              "SA2.MEDIUM4",
			"image_id":                   "img-xxx",
			"availability_zone":          "ap-guangzhou-6",
			"internet_charge_type":       "BANDWIDTH_POSTPAID_BY_HOUR",
			"internet_max_bandwidth_out": 10,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("CVM Extract: %v", err)
	}
	raw := []byte(`{"Response":{"Price":{"InstancePrice":{"UnitPriceDiscount":0.5,"ChargeUnit":"HOUR","Currency":"CNY"},"BandwidthPrice":{"UnitPrice":0.2,"ChargeUnit":"HOUR"}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("CVM Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("CVM components = %d, want 2 (compute+bandwidth)", len(comps))
	}
	bw := comps[1]
	if bw.Name != "Public bandwidth" {
		t.Fatalf("second component = %q, want Public bandwidth", bw.Name)
	}
	wantMonthly := 0.2 * hoursPerMonth
	if bw.MonthlyCost != wantMonthly {
		t.Errorf("bandwidth monthly = %v, want %v", bw.MonthlyCost, wantMonthly)
	}
	if bw.HourlyCost != 0.2 {
		t.Errorf("bandwidth hourly = %v, want 0.2", bw.HourlyCost)
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
	// POSTPAID hourly 0.1元 → monthly 0.1×730.
	if comps[0].HourlyCost != 0.1 {
		t.Errorf("CBS hourly = %v, want 0.1", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.1*hoursPerMonth {
		t.Errorf("CBS monthly = %v, want %v", comps[0].MonthlyCost, 0.1*hoursPerMonth)
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

// TestCLBLcuPrice verifies the LCU pricing dimension is parsed when present
// (LCU-style performance-guaranteed CLB).
func TestCLBLcuPrice(t *testing.T) {
	m := CLBInstance{}
	req := pricing.PriceRequest{
		Product: "clb",
		Action:  "InquiryPriceCreateLoadBalancer",
		Region:  "ap-beijing",
		Params:  map[string]interface{}{"LoadBalancerType": "OPEN"},
	}
	// Instance 0.3元/h + LCU 0.05元/h; no bandwidth.
	raw := clbInquiryPriceFullResp(t, "HOUR", 0.3, 0, 0.05)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("CLB Parse: %v", err)
	}
	// Expect 2 components: instance, LCU (bandwidth omitted — zero price).
	if len(comps) != 2 {
		t.Fatalf("CLB components = %d, want 2", len(comps))
	}
	wantHourly := map[string]float64{
		"CLB (OPEN)": 0.3,
		"CLB LCU":    0.05,
	}
	for _, c := range comps {
		want, ok := wantHourly[c.Name]
		if !ok {
			t.Errorf("unexpected component %q", c.Name)
			continue
		}
		if c.HourlyCost != want {
			t.Errorf("component %s hourly = %v, want %v", c.Name, c.HourlyCost, want)
		}
		if c.MonthlyCost != want*hoursPerMonth {
			t.Errorf("component %s monthly = %v, want %v", c.Name, c.MonthlyCost, want*hoursPerMonth)
		}
	}
}

// TestCLBBandwidthByTraffic verifies that a CLB response whose BandwidthPrice
// has ChargeUnit "GB" (traffic-based billing, 元/GB) is shown as a rate only
// with zero monthly/hourly estimates (monthly cost depends on traffic volume,
// which is not available from the plan).
func TestCLBBandwidthByTraffic(t *testing.T) {
	m := CLBInstance{}
	req := pricing.PriceRequest{
		Product: "clb",
		Action:  "InquiryPriceCreateLoadBalancer",
		Region:  "ap-beijing",
		Params:  map[string]interface{}{"LoadBalancerType": "OPEN"},
	}
	// Instance 0.2元/h (HOUR) + bandwidth 0.8元/GB (GB).
	t.Helper()
	type itemPrice struct {
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		ChargeUnit        string  `json:"ChargeUnit"`
	}
	type priceShape struct {
		InstancePrice  itemPrice `json:"InstancePrice"`
		BandwidthPrice itemPrice `json:"BandwidthPrice"`
	}
	type respShape struct {
		Price    priceShape `json:"Price"`
		Response struct {
			Price priceShape `json:"Price"`
		} `json:"Response"`
	}
	p := priceShape{
		InstancePrice:  itemPrice{UnitPriceDiscount: 0.2, ChargeUnit: "HOUR"},
		BandwidthPrice: itemPrice{UnitPriceDiscount: 0.8, ChargeUnit: "GB"},
	}
	resp := respShape{Price: p}
	resp.Response.Price = p
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("CLB Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("CLB components = %d, want 2", len(comps))
	}
	// First component: instance, billed hourly.
	if comps[0].Name != "CLB (OPEN)" {
		t.Errorf("component 0 name = %q, want CLB (OPEN)", comps[0].Name)
	}
	if comps[0].HourlyCost != 0.2 {
		t.Errorf("instance hourly = %v, want 0.2", comps[0].HourlyCost)
	}
	// Second component: bandwidth by traffic (GB) — rate only, no monthly/hourly.
	if comps[1].Unit != "GB" {
		t.Errorf("bandwidth unit = %q, want GB", comps[1].Unit)
	}
	if comps[1].HourlyCost != 0 || comps[1].MonthlyCost != 0 {
		t.Errorf("bandwidth by traffic should have zero hourly/monthly, got %v/%v",
			comps[1].HourlyCost, comps[1].MonthlyCost)
	}
}

// BandwidthPrice.UnitPrice but leaves UnitPriceDiscount at 0 still produces a
// non-zero bandwidth cost.
func TestCLBBandwidthUnitPriceFallback(t *testing.T) {
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
	raw := []byte(`{"Response":{"Price":{"InstancePrice":{"UnitPriceDiscount":0.3,"ChargeUnit":"HOUR","Currency":"CNY"},"BandwidthPrice":{"UnitPrice":0.15,"ChargeUnit":"HOUR"}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("CLB Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("CLB components = %d, want 2 (instance+bandwidth)", len(comps))
	}
	bw := comps[1]
	if bw.Name != "CLB bandwidth" {
		t.Fatalf("second component = %q, want CLB bandwidth", bw.Name)
	}
	wantMonthly := 0.15 * hoursPerMonth
	if bw.MonthlyCost != wantMonthly {
		t.Errorf("bandwidth monthly = %v, want %v", bw.MonthlyCost, wantMonthly)
	}
	if bw.HourlyCost != 0.15 {
		t.Errorf("bandwidth hourly = %v, want 0.15", bw.HourlyCost)
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

// TestPostgreSQLNormalizesPostpaid verifies that the TF charge_type
// "POSTPAID_BY_HOUR" is normalized to the API enum "POSTPAID".
func TestPostgreSQLNormalizesPostpaid(t *testing.T) {
	req, err := PostgreSQLInstance{}.Extract(parser.PlannedResource{
		Type:   "tencentcloud_postgresql_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone":    "ap-guangzhou-3",
			"spec_code":            "pg.it2.large",
			"storage":              100,
			"instance_charge_type": "POSTPAID_BY_HOUR",
		},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Params["InstanceChargeType"] != "POSTPAID" {
		t.Fatalf("InstanceChargeType = %v, want POSTPAID", req.Params["InstanceChargeType"])
	}
}

// TestPostgreSQLPostpaidHourly verifies POSTPAID pricing: the API returns
// the hourly rate in 分; Parse must convert to 元/hour and ×730 for monthly.
func TestPostgreSQLPostpaidHourly(t *testing.T) {
	m := PostgreSQLInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_postgresql_instance.pg",
		Type:    "tencentcloud_postgresql_instance",
		Region:  "ap-beijing",
		After: map[string]interface{}{
			"availability_zone":    "ap-beijing-3",
			"spec_code":            "pg.it2.large",
			"storage":              100,
			"instance_charge_type": "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Postgres Extract: %v", err)
	}
	// POSTPAID: Price=30 (分) = 0.3 元/hour.
	raw := []byte(`{"Response":{"Price":30,"OriginalPrice":30,"Currency":"CNY"}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Postgres Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if comps[0].HourlyCost != 0.3 {
		t.Errorf("hourly = %v, want 0.3", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.3*hoursPerMonth {
		t.Errorf("monthly = %v, want %v", comps[0].MonthlyCost, 0.3*hoursPerMonth)
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

	// DescribeDBPrice response (price in cents).
	raw := []byte(`{"Response":{"Price":50000,"Currency":"CNY"}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("MySQL Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	// POSTPAID: hourly = 500CNY(cents→CNY), monthly = 500 * 730
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

	// Response.Price.InstancePrice is in CNY; UnitPrice is CNY/hour for POSTPAID.
	raw := []byte(`{"Response":{"Price":{"InstancePrice":{"UnitPrice":0.5,"ChargeUnit":"HOUR"}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("VPN Parse: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("VPN returned 0 components")
	}
	// POSTPAID hourly 0.5CNY → monthly 0.5*730.
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

// TestVPNGatewayPostpaidByTraffic verifies the POSTPAID_BY_HOUR response where
// the instance is billed per-hour (HOUR) and bandwidth is billed per-GB
// (traffic-based). The bandwidth line must show the rate only with zero
// monthly/hourly estimates (per API doc example 2).
func TestVPNGatewayPostpaidByTraffic(t *testing.T) {
	m := VPNGateway{}
	r := parser.PlannedResource{
		Address: "tencentcloud_vpn_gateway.gw",
		Type:    "tencentcloud_vpn_gateway",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"bandwidth":   5,
			"charge_type": "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("VPN Extract: %v", err)
	}
	// Mirrors a real API POSTPAID_BY_HOUR response:
	// InstancePrice: UnitPrice=0.48 (原价), DiscountPrice=0.228 (折后价元/hour), ChargeUnit="HOUR"
	// BandwidthPrice: UnitPrice=0.8 (元/GB), DiscountPrice=0.374 (折后元/GB), ChargeUnit="GB"
	raw := []byte(`{"Response":{"Price":{"InstancePrice":{"UnitPrice":0.48,"DiscountPrice":0.228,"OriginalPrice":0.48,"ChargeUnit":"HOUR"},"BandwidthPrice":{"UnitPrice":0.8,"DiscountPrice":0.374,"OriginalPrice":0.8,"ChargeUnit":"GB"}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("VPN Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("VPN components = %d, want 2", len(comps))
	}
	// Instance: hourly billing, 0.228 元/h (折后) → monthly 0.228×730.
	if comps[0].HourlyCost != 0.228 {
		t.Errorf("VPN instance hourly = %v, want 0.228", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.228*hoursPerMonth {
		t.Errorf("VPN instance monthly = %v, want %v", comps[0].MonthlyCost, 0.228*hoursPerMonth)
	}
	// Bandwidth: per-GB traffic, rate only — no monthly/hourly estimate.
	if comps[1].Unit != "GB" {
		t.Errorf("VPN bandwidth unit = %q, want GB", comps[1].Unit)
	}
	if comps[1].HourlyCost != 0 || comps[1].MonthlyCost != 0 {
		t.Errorf("VPN bandwidth by traffic should have zero hourly/monthly, got %v/%v",
			comps[1].HourlyCost, comps[1].MonthlyCost)
	}
}

// TestVPNGatewayPrepaidNone verifies the PREPAID response (empty ChargeUnit,
// period total). Uses a real API response captured from InquiryPriceCreateVpnGateway
// with InternetMaxBandwidthOut=10, InstanceChargeType=PREPAID, Period=1.
func TestVPNGatewayPrepaidNone(t *testing.T) {
	m := VPNGateway{}
	req := pricing.PriceRequest{
		Product: "vpc",
		Action:  "InquiryPriceCreateVpnGateway",
		Region:  "ap-guangzhou",
		Params:  map[string]interface{}{"InternetMaxBandwidthOut": 10, "InstanceChargeType": "PREPAID"},
	}
	// Real API response (PREPAID, Period=1): instance 427.86 元 (1-month total,
	// ChargeUnit is empty string, NOT "none" as docs suggest), bandwidth 0.
	raw := []byte(`{"Response":{"Price":{"BandwidthPrice":{"ChargeUnit":"","DiscountPrice":0,"OriginalPrice":0,"UnitPrice":0},"InstancePrice":{"ChargeUnit":"","DiscountPrice":427.86,"OriginalPrice":880,"UnitPrice":0}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("VPN Parse: %v", err)
	}
	// Only instance component (bandwidth is 0 → skipped).
	if len(comps) != 1 {
		t.Fatalf("VPN components = %d, want 1 (bandwidth is 0)", len(comps))
	}
	// PREPAID empty ChargeUnit: DiscountPrice is the period (Period=1 → monthly) total.
	if comps[0].MonthlyCost != 427.86 {
		t.Errorf("VPN instance monthly = %v, want 427.86", comps[0].MonthlyCost)
	}
	if comps[0].HourlyCost != 0 {
		t.Errorf("VPN PREPAID should have zero hourly, got %v", comps[0].HourlyCost)
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

	// PREPAID: DiscountPrice is a period (monthly) total in CNY.
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
	// POSTPAID: UnitPrice is CNY/hour.
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

	// PREPAID DescribePrice returns cents (period total); 15000cents = 150CNY.
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
	// POSTPAID: 50cents/h = 0.5CNY/hour.
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

	// PREPAID: TotalPriceDiscount is cents. instance 20000cents=200CNY + storage 5000cents=50CNY.
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
			"charge_type":    "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Cynos Extract: %v", err)
	}
	// POSTPAID: UnitPriceDiscount is cents/h. 100cents/h = 1CNY/hour.
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

// TestCynosDBServerless verifies SERVERLESS pay mode: uses db_mode=SERVERLESS
// with min_cpu/max_cpu (mapped to API's Ccu), and computes hourly cost from
// UnitPriceDiscount.
func TestCynosDBServerless(t *testing.T) {
	m := CynosDBCluster{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_cynosdb_cluster",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"db_mode":        "SERVERLESS",
			"min_cpu":        1,
			"max_cpu":        2,
			"charge_type":    "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Cynos Extract: %v", err)
	}
	if req.Params["InstancePayMode"] != "SERVERLESS" {
		t.Fatalf("InstancePayMode = %v, want SERVERLESS", req.Params["InstancePayMode"])
	}
	if ccu, ok := req.Params["Ccu"].(int64); !ok || ccu != 2 {
		t.Errorf("Ccu = %v, want int64(2) (from max_cpu)", req.Params["Ccu"])
	}
	if _, ok := req.Params["Cpu"]; ok {
		t.Error("Cpu should not be set for SERVERLESS")
	}
	// SERVERLESS is POSTPAID: UnitPriceDiscount is hourly rate (cents).
	// Instance 50 cents/h = 0.5 元/h; storage GB*h 350 cents = 3.5 元/GB·h.
	raw := []byte(`{"Response":{"InstancePrice":{"UnitPrice":60,"UnitPriceDiscount":50,"ChargeUnit":"HOUR"},"StoragePrice":{"UnitPrice":350,"UnitPriceDiscount":350,"ChargeUnit":"GB*h"}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Cynos Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("Cynos components = %d, want 2", len(comps))
	}
	// Instance: 50 cents/h = 0.5 元/hour.
	if comps[0].HourlyCost != 0.5 {
		t.Errorf("Cynos SERVERLESS instance hourly = %v, want 0.5", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.5*hoursPerMonth {
		t.Errorf("Cynos SERVERLESS instance monthly = %v, want %v", comps[0].MonthlyCost, 0.5*hoursPerMonth)
	}
	// Storage: GB*h rate only (no monthly/hourly estimate without volume).
	if comps[1].Unit != "GB*H" {
		t.Errorf("Cynos storage unit = %q, want GB*H", comps[1].Unit)
	}
	if comps[1].HourlyCost != 0 || comps[1].MonthlyCost != 0 {
		t.Errorf("Cynos storage GB*h should have zero hourly/monthly, got %v/%v",
			comps[1].HourlyCost, comps[1].MonthlyCost)
	}
}

// TestCynosDBPostpaidStorageGBh verifies that POSTPAID storage with ChargeUnit
// "GB*h" is shown as a rate only (per-GB-per-hour), without estimating monthly.
func TestCynosDBPostpaidStorageGBh(t *testing.T) {
	m := CynosDBCluster{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_cynosdb_cluster",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"cpu":            2,
			"memory":         4,
			"storage_limit":  100,
			"charge_type":    "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Cynos Extract: %v", err)
	}
	// Instance POSTPAID hourly + Storage POSTPAID GB*h.
	raw := []byte(`{"Response":{"InstancePrice":{"UnitPrice":120,"UnitPriceDiscount":100,"ChargeUnit":"HOUR"},"StoragePrice":{"UnitPrice":350,"UnitPriceDiscount":350,"ChargeUnit":"GB*h"}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Cynos Parse: %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("Cynos components = %d, want 2", len(comps))
	}
	// Instance: 100 cents/h = 1.0 元/hour.
	if comps[0].HourlyCost != 1.0 {
		t.Errorf("Cynos instance hourly = %v, want 1.0", comps[0].HourlyCost)
	}
	// Storage: GB*h rate only.
	if comps[1].Unit != "GB*H" {
		t.Errorf("Cynos storage unit = %q, want GB*H", comps[1].Unit)
	}
}

// TestPrepaidPricesSingleMonth guards against the period-total inflation bug:
// even when the user configures a multi-month prepaid term, the pricing request
// must ask for a single month so the returned price equals the monthly cost.
// TestPrepaidPricesSingleMonth enforces the universal invariant that a PREPAID
// Tencent instance is always priced as exactly one month of run-rate — never a
// multi-month total. This is the regression net for the bug that hid in six
// mappers for several releases.
//
// Unlike the original hand-written test (which pinned nine specific types), this
// version is table-driven over EVERY prepaid-capable Tencent type and asserts
// the rule generically: every "Period" (top-level or nested in a *ChargePrepaid
// map) and every "TimeSpan" found anywhere in the request must equal 1. Adding a
// new prepaid type = one line in the table; the invariant is checked uniformly.
func TestPrepaidPricesSingleMonth(t *testing.T) {
	cases := []struct {
		name string
		res  parser.PlannedResource
	}{
		{"cvm", parser.PlannedResource{Type: "tencentcloud_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"instance_type": "S5.LARGE8", "image_id": "img-xxx", "availability_zone": "ap-guangzhou-6",
			"instance_charge_type": "PREPAID", "instance_charge_type_prepaid_period": 12}}},
		{"cbs", parser.PlannedResource{Type: "tencentcloud_cbs_storage", Region: "ap-guangzhou", After: map[string]interface{}{
			"storage_type": "CLOUD_PREMIUM", "storage_size": 100, "availability_zone": "ap-guangzhou-6",
			"charge_type": "PREPAID", "prepaid_period": 12}}},
		{"mysql", parser.PlannedResource{Type: "tencentcloud_mysql_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-6", "mem_size": 4000, "volume_size": 200,
			"charge_type": "PREPAID", "prepaid_period": 12}}},
		{"postgresql", parser.PlannedResource{Type: "tencentcloud_postgresql_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "spec_code": "pg.it2.large", "storage": 100,
			"instance_charge_type": "PREPAID", "prepaid_period": 12}}},
		{"redis", parser.PlannedResource{Type: "tencentcloud_redis_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "mem_size": 1024,
			"charge_type": "PREPAID", "prepaid_period": 12}}},
		{"vpn", parser.PlannedResource{Type: "tencentcloud_vpn_gateway", Region: "ap-guangzhou", After: map[string]interface{}{
			"bandwidth": 100, "charge_type": "PREPAID", "prepaid_period": 12}}},
		{"mongodb", parser.PlannedResource{Type: "tencentcloud_mongodb_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3", "memory": 4, "volume": 100,
			"charge_type": "PREPAID", "prepaid_period": 12}}},
		{"mariadb", parser.PlannedResource{Type: "tencentcloud_mariadb_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "memory": 8, "storage": 200,
			"charge_type": "PREPAID", "period": 36}}},
		{"cynosdb", parser.PlannedResource{Type: "tencentcloud_cynosdb_cluster", Region: "ap-guangzhou", After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3", "cpu": 2, "memory": 4, "storage_limit": 100,
			"charge_type": "PREPAID", "prepaid_period": 24}}},
		{"lighthouse", parser.PlannedResource{Type: "tencentcloud_lighthouse_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"bundle_id": "bundle_xxx", "instance_count": 1}}},
		{"sqlserver", parser.PlannedResource{Type: "tencentcloud_sqlserver_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3", "memory": 8, "storage": 200,
			"charge_type": "PREPAID", "prepaid_period": 12}}},
		{"dcdb", parser.PlannedResource{Type: "tencentcloud_dcdb_db_instance", Region: "ap-guangzhou", After: map[string]interface{}{
			"zones": []interface{}{"ap-guangzhou-3"}, "shard_memory": 8, "shard_storage": 200, "shard_count": 2,
			"period": 24}}},
		{"yunjing", parser.PlannedResource{Type: "tencentcloud_cwp_license_order", Region: "ap-guangzhou", After: map[string]interface{}{}}},
		{"cloudhsm", parser.PlannedResource{Type: "tencentcloud_cloudhsm_instance", Region: "ap-guangzhou", After: map[string]interface{}{}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, ok := DefaultRegistry().Lookup(tc.res.Type)
			if !ok {
				t.Fatalf("type %s not registered", tc.res.Type)
			}
			req, err := entry.Extract(tc.res)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			periods, timeSpans, timeUnits := collectPeriodFields(req.Params)
			if len(periods) == 0 && len(timeSpans) == 0 {
				t.Fatalf("no Period/TimeSpan found in request for %s — prepaid term not asserted", tc.name)
			}
			for _, p := range periods {
				if p != 1 {
					t.Errorf("%s: Period = %v, want 1 (multi-month must not leak)", tc.name, p)
				}
			}
			for _, ts := range timeSpans {
				if ts != 1 {
					t.Errorf("%s: TimeSpan = %v, want 1", tc.name, ts)
				}
			}
			if len(timeSpans) > 0 && !containsStr(timeUnits, "m") {
				t.Errorf("%s: TimeSpan present but TimeUnit != \"m\" (%v)", tc.name, timeUnits)
			}
		})
	}
}

// collectPeriodFields walks a request-params map recursively and returns every
// Period value, every TimeSpan value, and every TimeUnit string found (at any
// nesting depth). Values are normalized to int so int/int64/string "1" all
// compare as 1.
func collectPeriodFields(params map[string]interface{}) (periods, timeSpans []int, timeUnits []string) {
	var walk func(m map[string]interface{})
	walk = func(m map[string]interface{}) {
		for k, v := range m {
			switch k {
			case "Period":
				if n, ok := toInt(v); ok {
					periods = append(periods, n)
				}
			case "TimeSpan":
				if n, ok := toInt(v); ok {
					timeSpans = append(timeSpans, n)
				}
			case "TimeUnit":
				if s, ok := v.(string); ok {
					timeUnits = append(timeUnits, s)
				}
			}
			if nested, ok := v.(map[string]interface{}); ok {
				walk(nested)
			}
		}
	}
	walk(params)
	return periods, timeSpans, timeUnits
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		if n == "1" {
			return 1, true
		}
	}
	return 0, false
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// ----- Lighthouse integration test -----

func TestLighthouseExtractAndParse(t *testing.T) {
	m := LighthouseInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_lighthouse_instance.lh",
		Type:    "tencentcloud_lighthouse_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"bundle_id":      "bundle_gen_01",
			"blueprint_id":   "lhbp-xxx",
			"instance_count": 1,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Lighthouse Extract: %v", err)
	}
	if req.Product != "lighthouse" || req.Action != "InquirePriceCreateInstances" {
		t.Fatalf("Lighthouse product/action = %q/%q", req.Product, req.Action)
	}
	// Period must always be a single month.
	prepaid, _ := req.Params["InstanceChargePrepaid"].(map[string]interface{})
	if prepaid == nil || prepaid["Period"] != 1 {
		t.Errorf("Lighthouse Period = %v, want 1", prepaid)
	}

	// Price.InstancePrice is a monthly total in CNY.
	raw := []byte(`{"Response":{"Price":{"InstancePrice":{"OriginalPrice":50,"DiscountPrice":45}}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Lighthouse Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("Lighthouse components = %d, want 1", len(comps))
	}
	if comps[0].MonthlyCost != 45 {
		t.Errorf("Lighthouse monthly = %v, want 45 (discounted)", comps[0].MonthlyCost)
	}
	if comps[0].HourlyCost != 0 {
		t.Errorf("Lighthouse hourly = %v, want 0 (prepaid)", comps[0].HourlyCost)
	}
}

// ----- ECM integration test -----

func TestECMExtractAndParse(t *testing.T) {
	m := ECMInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_ecm_instance.edge",
		Type:    "tencentcloud_ecm_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"instance_type":    "ec.small1.medium2",
			"instance_count":   1,
			"system_disk_size": 50,
			"system_disk_type": "CLOUD_PREMIUM",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("ECM Extract: %v", err)
	}
	if req.Product != "ecm" || req.Action != "DescribePriceRunInstance" {
		t.Fatalf("ECM product/action = %q/%q", req.Product, req.Action)
	}
	// SystemDisk must be a nested object with the disk size.
	disk, _ := req.Params["SystemDisk"].(map[string]interface{})
	if disk == nil || disk["DiskSize"] != int64(50) {
		t.Errorf("ECM SystemDisk = %v, want DiskSize 50", req.Params["SystemDisk"])
	}

	// InstancePrice is uint64 cents; 120cents = 1.2CNY/hour.
	raw := []byte(`{"Response":{"InstancePrice":{"OriginalPrice":150,"DiscountPrice":120}}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("ECM Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("ECM components = %d, want 1", len(comps))
	}
	if comps[0].HourlyCost != 1.2 {
		t.Errorf("ECM hourly = %v, want 1.2", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 1.2*hoursPerMonth {
		t.Errorf("ECM monthly = %v, want %v", comps[0].MonthlyCost, 1.2*hoursPerMonth)
	}
}

// ----- SQL Server integration test -----

func TestSQLServerExtractAndParse(t *testing.T) {
	m := SQLServerInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_sqlserver_instance.mssql",
		Type:    "tencentcloud_sqlserver_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-3",
			"memory":            4,
			"storage":           100,
			"charge_type":       "PREPAID",
			"prepaid_period":    12,
			"cpu":               2,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("SQLServer Extract: %v", err)
	}
	if req.Product != "sqlserver" || req.Action != "InquiryPriceCreateDBInstances" {
		t.Fatalf("SQLServer product/action = %q/%q", req.Product, req.Action)
	}
	// Even with prepaid_period=12, Period must be forced to a single month.
	if req.Params["Period"] != 1 {
		t.Errorf("SQLServer Period = %v, want 1", req.Params["Period"])
	}

	// PREPAID: Price is int64 cents (monthly total); 15000cents = 150CNY.
	raw := []byte(`{"Response":{"Price":15000,"OriginalPrice":20000}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("SQLServer Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("SQLServer components = %d, want 1", len(comps))
	}
	if comps[0].MonthlyCost != 150 {
		t.Errorf("SQLServer monthly = %v, want 150", comps[0].MonthlyCost)
	}
}

func TestSQLServerPostpaidHourly(t *testing.T) {
	m := SQLServerInstance{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_sqlserver_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"zone":        "ap-guangzhou-3",
			"memory":      4,
			"storage":     100,
			"charge_type": "POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("SQLServer Extract: %v", err)
	}
	if req.Params["InstanceChargeType"] != "POSTPAID" {
		t.Errorf("SQLServer charge type = %v, want POSTPAID", req.Params["InstanceChargeType"])
	}
	// POSTPAID: 50cents/h = 0.5CNY/hour.
	raw := []byte(`{"Response":{"Price":50}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("SQLServer Parse: %v", err)
	}
	if comps[0].HourlyCost != 0.5 {
		t.Errorf("SQLServer hourly = %v, want 0.5", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.5*hoursPerMonth {
		t.Errorf("SQLServer monthly = %v, want %v", comps[0].MonthlyCost, 0.5*hoursPerMonth)
	}
}

// ----- DCDB (TDSQL MySQL) integration test -----

func TestDCDBExtractAndParse(t *testing.T) {
	m := DCDBInstance{}
	r := parser.PlannedResource{
		Address: "tencentcloud_dcdb_db_instance.tdsql",
		Type:    "tencentcloud_dcdb_db_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"zones":            []interface{}{"ap-guangzhou-3", "ap-guangzhou-4"},
			"shard_memory":     8,
			"shard_storage":    200,
			"shard_count":      2,
			"shard_node_count": 2,
			"period":           12,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("DCDB Extract: %v", err)
	}
	if req.Product != "dcdb" || req.Action != "DescribeDCDBPrice" {
		t.Fatalf("DCDB product/action = %q/%q", req.Product, req.Action)
	}
	if req.Params["Zone"] != "ap-guangzhou-3" {
		t.Errorf("DCDB Zone = %v, want first of zones list", req.Params["Zone"])
	}
	if req.Params["Paymode"] != "prepaid" {
		t.Errorf("DCDB Paymode = %v, want prepaid", req.Params["Paymode"])
	}
	if req.Params["Period"] != 1 {
		t.Errorf("DCDB Period = %v, want 1 (multi-month must not leak)", req.Params["Period"])
	}

	// PREPAID: Price is int64 cents (monthly total); 30000cents = 300CNY.
	raw := []byte(`{"Response":{"Price":30000,"OriginalPrice":40000}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("DCDB Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("DCDB components = %d, want 1", len(comps))
	}
	if comps[0].MonthlyCost != 300 {
		t.Errorf("DCDB monthly = %v, want 300", comps[0].MonthlyCost)
	}
}

func TestDCDBPostpaidHourly(t *testing.T) {
	m := DCDBInstance{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_dcdb_hourdb_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"zones":         []interface{}{"ap-guangzhou-3"},
			"shard_memory":  8,
			"shard_storage": 200,
			"shard_count":   2,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("DCDB Extract: %v", err)
	}
	if req.Params["Paymode"] != "postpaid" {
		t.Errorf("DCDB Paymode = %v, want postpaid", req.Params["Paymode"])
	}
	// POSTPAID: 60cents/h = 0.6CNY/hour.
	raw := []byte(`{"Response":{"Price":60}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("DCDB Parse: %v", err)
	}
	if comps[0].HourlyCost != 0.6 {
		t.Errorf("DCDB hourly = %v, want 0.6", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.6*hoursPerMonth {
		t.Errorf("DCDB monthly = %v, want %v", comps[0].MonthlyCost, 0.6*hoursPerMonth)
	}
}

// ----- GAAP proxy integration test -----

func TestGAAPExtractAndParse(t *testing.T) {
	m := GAAPProxy{}
	r := parser.PlannedResource{
		Address: "tencentcloud_gaap_proxy.acc",
		Type:    "tencentcloud_gaap_proxy",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"access_region":     "Guangzhou",
			"realserver_region": "Beijing",
			"bandwidth":         10,
			"concurrent":        2,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("GAAP Extract: %v", err)
	}
	if req.Product != "gaap" || req.Action != "InquiryPriceCreateProxy" {
		t.Fatalf("GAAP product/action = %q/%q", req.Product, req.Action)
	}

	// Daily price in CNY; monthly = daily * (730/24).
	raw := []byte(`{"Response":{"ProxyDailyPrice":12,"DiscountProxyDailyPrice":10}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("GAAP Parse: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("GAAP components = %d, want 1", len(comps))
	}
	wantMonthly := 10.0 * daysPerMonth
	if comps[0].MonthlyCost != wantMonthly {
		t.Errorf("GAAP monthly = %v, want %v (discounted daily * days)", comps[0].MonthlyCost, wantMonthly)
	}
}

func TestGAAPConcurrentPassthrough(t *testing.T) {
	m := GAAPProxy{}
	r := parser.PlannedResource{
		Type:   "tencentcloud_gaap_proxy",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"access_region":     "Guangzhou",
			"realserver_region": "Beijing",
			"bandwidth":         10,
			// Terraform stores concurrent already in units of 10k; pass through.
			"concurrent": 5,
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("GAAP Extract: %v", err)
	}
	if req.Params["Concurrent"] != int64(5) {
		t.Errorf("GAAP Concurrent = %v, want 5 (passed through unchanged)", req.Params["Concurrent"])
	}
	// billing_type defaults to 0 (by bandwidth) when absent.
	if req.Params["BillingType"] != 0 {
		t.Errorf("GAAP BillingType = %v, want 0 (default by-bandwidth)", req.Params["BillingType"])
	}
}

func TestGAAPBillingTypeFlow(t *testing.T) {
	base := map[string]interface{}{
		"access_region":     "Guangzhou",
		"realserver_region": "Beijing",
		"bandwidth":         10,
		"concurrent":        2,
	}
	// Numeric 1 and string "flow" must both map to BillingType=1 (by flow).
	for _, bt := range []interface{}{1, "flow", "1"} {
		after := map[string]interface{}{}
		for k, v := range base {
			after[k] = v
		}
		after["billing_type"] = bt
		req, err := (GAAPProxy{}).Extract(parser.PlannedResource{
			Type: "tencentcloud_gaap_proxy", Region: "ap-guangzhou", After: after,
		})
		if err != nil {
			t.Fatalf("GAAP Extract (billing_type=%v): %v", bt, err)
		}
		if req.Params["BillingType"] != 1 {
			t.Errorf("GAAP BillingType (input %v) = %v, want 1 (by flow)", bt, req.Params["BillingType"])
		}
	}
}

// ----- EIP integration test -----

func TestEIPExtractAndParse(t *testing.T) {
	m := EIP{}
	r := parser.PlannedResource{
		Address: "tencentcloud_eip.pub",
		Type:    "tencentcloud_eip",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"internet_max_bandwidth_out": 10,
			"internet_charge_type":       "TRAFFIC_POSTPAID_BY_HOUR",
		},
	}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("EIP Extract: %v", err)
	}
	raw := eipInquiryPriceResp(t, "HOUR", 0.5, 0.5)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("EIP Parse: %v", err)
	}
	if len(comps) == 0 {
		t.Fatal("EIP returned 0 components")
	}
	// POSTPAID hourly 0.5元 → monthly 0.5×730.
	if comps[0].HourlyCost != 0.5 {
		t.Errorf("EIP hourly = %v, want 0.5", comps[0].HourlyCost)
	}
	if comps[0].MonthlyCost != 0.5*hoursPerMonth {
		t.Errorf("EIP monthly = %v, want %v", comps[0].MonthlyCost, 0.5*hoursPerMonth)
	}
}

// ----- Registry lookup test -----

// expectedAllTypes is the canonical, exhaustive set of resource types that must
// be registered. Ordered so the list is reviewable. This guards against two
// failure modes that a partial list misses:
//  1. a mapper is implemented but forgotten in DefaultRegistry (lookup fails);
//  2. the registry drifts (a type removed, or an extra/duplicate registered).
//
// If you add a new mapper, you MUST update this list — that friction is the point.
var expectedAllTypes = []string{
	// Alibaba Cloud (9)
	"alicloud_db_instance",
	"alicloud_disk",
	"alicloud_eip",
	"alicloud_instance",
	"alicloud_kvstore_instance",
	"alicloud_mongodb_instance",
	"alicloud_nat_gateway",
	"alicloud_slb_load_balancer",
	"alicloud_vpn_gateway",
	// AWS (21)
	"aws_db_instance",
	"aws_docdb_cluster_instance",
	"aws_dynamodb_table",
	"aws_ebs_volume",
	"aws_eks_cluster",
	"aws_elasticache_cluster",
	"aws_elasticsearch_domain",
	"aws_elb",
	"aws_instance",
	"aws_lb",
	"aws_memorydb_cluster",
	"aws_mq_broker",
	"aws_msk_cluster",
	"aws_nat_gateway",
	"aws_neptune_cluster_instance",
	"aws_opensearch_domain",
	"aws_rds_cluster_instance",
	"aws_redshift_cluster",
	// Huawei Cloud (9)
	"huaweicloud_cce_cluster",
	"huaweicloud_compute_instance",
	"huaweicloud_dcs_instance",
	"huaweicloud_dds_instance",
	"huaweicloud_elb_loadbalancer",
	"huaweicloud_evs_volume",
	"huaweicloud_nat_gateway",
	"huaweicloud_rds_instance",
	"huaweicloud_vpc_eip",
	// Tencent Cloud (23 — note: cwp_license_order == tencentcloud_cwp_license_order;
	// the 4 static usage-driven resources below have no InquiryPrice* API and are
	// registered as StaticMappers returning a zero-cost placeholder + note)
	"tencentcloud_cbs_storage",
	"tencentcloud_clb_instance",
	"tencentcloud_cloudhsm_instance",
	"tencentcloud_cwp_license_order",
	"tencentcloud_cynosdb_cluster",
	"tencentcloud_dcdb_db_instance",
	"tencentcloud_dcdb_hourdb_instance",
	"tencentcloud_dcdb_instance", // legacy alias
	"tencentcloud_domain_registration",
	"tencentcloud_ecm_instance",
	"tencentcloud_eip",
	"tencentcloud_gaap_proxy",
	"tencentcloud_instance",
	"tencentcloud_lighthouse_instance",
	"tencentcloud_mariadb_instance",
	"tencentcloud_mongodb_instance",
	"tencentcloud_mysql_instance",
	"tencentcloud_postgresql_instance",
	"tencentcloud_redis_instance",
	"tencentcloud_sqlserver_instance",
	"tencentcloud_vpn_gateway",
	"tencentcloud_dc_gateway",
	"tencentcloud_cos_bucket",
	"tencentcloud_cdn_domain",
	"tencentcloud_cfs_file_system",
	"tencentcloud_scf_function",
}

func TestDefaultRegistryHasAllTypes(t *testing.T) {
	reg := DefaultRegistry()

	// Forward guard: every expected type must be registered.
	got := make(map[string]bool, len(expectedAllTypes))
	for _, typ := range expectedAllTypes {
		if _, ok := reg.Lookup(typ); !ok {
			t.Errorf("DefaultRegistry missing type: %s", typ)
		}
		got[typ] = true
	}

	// Reverse guard: registry must contain exactly expectedAllTypes, nothing more
	// (catches forgotten removals and accidental extras/duplicates).
	for _, name := range reg.Keys() {
		if !got[name] {
			t.Errorf("DefaultRegistry contains unexpected/duplicate type not in expectedAllTypes: %s", name)
		}
	}
	if reg.Len() != len(expectedAllTypes) {
		t.Errorf("DefaultRegistry size = %d, want %d (expectedAllTypes)", reg.Len(), len(expectedAllTypes))
	}
}

// ----- mock response helpers -----

// inquiryPriceRunInstancesResp builds a fake CVM InquiryPriceRunInstances
// response JSON that matches the real SDK ToJsonString() output: the price
// data is nested under "Response.Price.InstancePrice" (and "Response.Price.
// BandwidthPrice" when bandwidth > 0). A top-level mirror is also included
// for backward compatibility with older test paths.
func inquiryPriceRunInstancesResp(t *testing.T, chargeUnit string, unitPriceDiscount, discountPrice float64) []byte {
	t.Helper()
	type itemPrice struct {
		UnitPrice         float64 `json:"UnitPrice"`
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		OriginalPrice     float64 `json:"OriginalPrice"`
		DiscountPrice     float64 `json:"DiscountPrice"`
		ChargeUnit        string  `json:"ChargeUnit"`
	}
	type priceStruct struct {
		InstancePrice itemPrice `json:"InstancePrice"`
	}
	type respShape struct {
		Price    priceStruct `json:"Price"`
		Response struct {
			Price priceStruct `json:"Price"`
		} `json:"Response"`
	}
	resp := respShape{
		Price: priceStruct{
			InstancePrice: itemPrice{
				UnitPriceDiscount: unitPriceDiscount,
				DiscountPrice:     discountPrice,
				ChargeUnit:        chargeUnit,
			},
		},
	}
	resp.Response.Price = resp.Price
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal mock response: %v", err)
	}
	return b
}

// inquiryPriceCreateDisksResp builds a fake CBS InquiryPriceCreateDisks
// response JSON that matches the real SDK ToJsonString() output: the disk
// price is nested under "Response.DiskPrice". A top-level mirror is also
// included for backward compatibility with older test paths.
func inquiryPriceCreateDisksResp(t *testing.T, chargeUnit string, unitPriceDiscount, discountPrice float64) []byte {
	t.Helper()
	type diskPrice struct {
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		DiscountPrice     float64 `json:"DiscountPrice"`
		ChargeUnit        string  `json:"ChargeUnit"`
	}
	type respShape struct {
		DiskPrice diskPrice `json:"DiskPrice"`
		Response  struct {
			DiskPrice diskPrice `json:"DiskPrice"`
		} `json:"Response"`
	}
	resp := respShape{
		DiskPrice: diskPrice{
			UnitPriceDiscount: unitPriceDiscount,
			DiscountPrice:     discountPrice,
			ChargeUnit:        chargeUnit,
		},
	}
	resp.Response.DiskPrice = resp.DiskPrice
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal mock response: %v", err)
	}
	return b
}

func inquiryPriceCreateLBResp(t *testing.T, chargeUnit string, unitPriceDiscount, discountPrice float64) []byte {
	// CLB uses the same Price.InstancePrice shape as CVM.
	return inquiryPriceRunInstancesResp(t, chargeUnit, unitPriceDiscount, discountPrice)
}

// clbInquiryPriceFullResp builds a CLB response carrying all three pricing
// dimensions (instance/bandwidth/LCU), each as an ItemPrice with the given
// discounted unit price and charge unit. Zero-value dimensions are emitted but
// Parse skips them (no non-zero price fields). Matches the real SDK
// ToJsonString() output: data is nested under "Response.Price".
func clbInquiryPriceFullResp(t *testing.T, chargeUnit string, inst, bw, lcu float64) []byte {
	t.Helper()
	type itemPrice struct {
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		ChargeUnit        string  `json:"ChargeUnit"`
	}
	type priceShape struct {
		InstancePrice  itemPrice `json:"InstancePrice"`
		BandwidthPrice itemPrice `json:"BandwidthPrice"`
		LcuPrice       itemPrice `json:"LcuPrice"`
	}
	type respShape struct {
		Price    priceShape `json:"Price"`
		Response struct {
			Price priceShape `json:"Price"`
		} `json:"Response"`
	}
	p := priceShape{
		InstancePrice:  itemPrice{UnitPriceDiscount: inst, ChargeUnit: chargeUnit},
		BandwidthPrice: itemPrice{UnitPriceDiscount: bw, ChargeUnit: chargeUnit},
		LcuPrice:       itemPrice{UnitPriceDiscount: lcu, ChargeUnit: chargeUnit},
	}
	resp := respShape{Price: p}
	resp.Response.Price = p
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal mock response: %v", err)
	}
	return b
}

// eipInquiryPriceResp builds a fake EIP InquiryPriceAllocateAddresses
// response JSON matching the real SDK ToJsonString() output. The price is
// nested under "Response.Price.AddressPrice"; a top-level mirror is included
// for backward compatibility.
func eipInquiryPriceResp(t *testing.T, chargeUnit string, discountPrice, unitPrice float64) []byte {
	t.Helper()
	type addressPrice struct {
		UnitPrice     float64 `json:"UnitPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
		ChargeUnit    string  `json:"ChargeUnit"`
	}
	type respShape struct {
		Price struct {
			AddressPrice addressPrice `json:"AddressPrice"`
		} `json:"Price"`
		Response struct {
			Price struct {
				AddressPrice addressPrice `json:"AddressPrice"`
			} `json:"Price"`
		} `json:"Response"`
	}
	resp := respShape{}
	resp.Price.AddressPrice = addressPrice{
		UnitPrice:     unitPrice,
		DiscountPrice: discountPrice,
		ChargeUnit:    chargeUnit,
	}
	resp.Response.Price.AddressPrice = resp.Price.AddressPrice
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal mock response: %v", err)
	}
	return b
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
		{
			addr:  "tencentcloud_lighthouse_instance.lh",
			typ:   "tencentcloud_lighthouse_instance",
			after: map[string]interface{}{"bundle_id": "bundle_gen_01", "instance_count": 1},
		},
		{
			addr:  "tencentcloud_ecm_instance.edge",
			typ:   "tencentcloud_ecm_instance",
			after: map[string]interface{}{"instance_type": "ec.small1.medium2", "instance_count": 1, "system_disk_size": 50},
		},
		{
			addr: "tencentcloud_sqlserver_instance.mssql",
			typ:  "tencentcloud_sqlserver_instance",
			after: map[string]interface{}{
				"availability_zone": "ap-guangzhou-3", "memory": 4, "storage": 100, "charge_type": "POSTPAID",
			},
		},
		{
			addr: "tencentcloud_dcdb_hourdb_instance.tdsql",
			typ:  "tencentcloud_dcdb_hourdb_instance",
			after: map[string]interface{}{
				"zones": []interface{}{"ap-guangzhou-3"}, "shard_memory": 8, "shard_storage": 200,
				"shard_count": 2,
			},
		},
		{
			addr: "tencentcloud_gaap_proxy.acc",
			typ:  "tencentcloud_gaap_proxy",
			after: map[string]interface{}{
				"access_region": "Guangzhou", "realserver_region": "Beijing", "bandwidth": 10, "concurrent": 2,
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
