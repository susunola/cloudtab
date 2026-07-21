package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

func TestRedisExtract(t *testing.T) {
	r := parser.PlannedResource{
		Address: "tencentcloud_redis_instance.cache",
		Type:    "tencentcloud_redis_instance",
		Region:  "ap-guangzhou",
		After: map[string]interface{}{
			"availability_zone":  "ap-guangzhou-6",
			"type_id":            7,
			"mem_size":           4096,
			"charge_type":        "POSTPAID",
			"redis_shard_num":    1,
			"redis_replicas_num": 2,
		},
	}
	m := RedisInstance{}
	req, err := m.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Product != "redis" || req.Action != "InquiryPriceCreateInstance" {
		t.Fatalf("unexpected route: %s/%s", req.Product, req.Action)
	}
	if got := req.Params["BillingMode"]; got != int64(0) {
		t.Fatalf("BillingMode = %v, want 0", got)
	}
	if got := req.Params["TypeId"]; got != int64(7) {
		t.Fatalf("TypeId = %v, want 7", got)
	}
}

func TestRedisParsePostpaid(t *testing.T) {
	m := RedisInstance{}
	req := pricing.PriceRequest{
		Product: "redis",
		Action:  "InquiryPriceCreateInstance",
		Region:  "ap-guangzhou",
		Params: map[string]interface{}{
			"BillingMode": int64(0),
			"TypeId":      int64(6),
			"MemSize":     int64(2048),
		},
	}
	raw := []byte(`{"Response":{"Price":280,"AmountUnit":"pent","Currency":"CNY"}}`)
	comps, err := m.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if !almostEqRedis(comps[0].HourlyCost, 2.8) {
		t.Fatalf("hourly = %v, want 2.8", comps[0].HourlyCost)
	}
	if !almostEqRedis(comps[0].MonthlyCost, 2.8*hoursPerMonth) {
		t.Fatalf("monthly = %v, want %v", comps[0].MonthlyCost, 2.8*hoursPerMonth)
	}
}

func TestNormalizeTencentAmount(t *testing.T) {
	if got := normalizeTencentAmount(280, "pent"); !almostEqRedis(got, 2.8) {
		t.Fatalf("pent conversion = %v, want 2.8", got)
	}
	if got := normalizeTencentAmount(280000000, "microPent"); !almostEqRedis(got, 2.8) {
		t.Fatalf("microPent conversion = %v, want 2.8", got)
	}
	if got := normalizeTencentAmount(2.8, ""); !almostEqRedis(got, 2.8) {
		t.Fatalf("default conversion = %v, want 2.8", got)
	}
}

func almostEqRedis(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
