package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

func TestMySQLExtract(t *testing.T) {
	r := parser.PlannedResource{
		Address: "tencentcloud_mysql_instance.db",
		Type:    "tencentcloud_mysql_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone": "ap-guangzhou-6",
			"mem_size":          4000,
			"volume_size":       200,
			"charge_type":       "PREPAID",
			"prepaid_period":    12,
			"cpu":               2,
		},
	}
	m := MySQLInstance{}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Product != "cdb" || req.Action != "DescribeDBPrice" {
		t.Fatalf("unexpected route: %s/%s", req.Product, req.Action)
	}
	if got := req.Params["PayType"]; got != "PRE_PAID" {
		t.Fatalf("PayType = %v, want PRE_PAID", got)
	}
	if got := req.Params["Memory"]; got != int64(4000) {
		t.Fatalf("Memory = %v, want 4000", got)
	}
	if got := req.Params["Volume"]; got != int64(200) {
		t.Fatalf("Volume = %v, want 200", got)
	}
}

func TestMySQLParsePrepaid(t *testing.T) {
	m := MySQLInstance{}
	req := mapReq("PRE_PAID")
	raw := []byte(`{"Response":{"Price":12345,"Currency":"CNY"}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if !almostEq(comps[0].MonthlyCost, 123.45) {
		t.Fatalf("monthly = %v, want 123.45", comps[0].MonthlyCost)
	}
	if comps[0].HourlyCost != 0 {
		t.Fatalf("hourly = %v, want 0", comps[0].HourlyCost)
	}
}

func TestMySQLParseHourPaid(t *testing.T) {
	m := MySQLInstance{}
	req := mapReq("HOUR_PAID")
	raw := []byte(`{"Response":{"Price":100,"Currency":"CNY"}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if !almostEq(comps[0].HourlyCost, 1.0) {
		t.Fatalf("hourly = %v, want 1.0", comps[0].HourlyCost)
	}
	if !almostEq(comps[0].MonthlyCost, 730.0) {
		t.Fatalf("monthly = %v, want 730.0", comps[0].MonthlyCost)
	}
}

func mapReq(payType string) pricing.PriceRequest {
	return pricing.PriceRequest{
		Product: "cdb",
		Action:  "DescribeDBPrice",
		Region:  "ap-guangzhou",
		Params: map[string]interface{}{
			"PayType": payType,
			"Memory":  int64(4000),
			"Volume":  int64(200),
		},
	}
}

func almostEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
