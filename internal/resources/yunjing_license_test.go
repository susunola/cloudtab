package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

func almostEqYunjing(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestYunjingExtract(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "tencentcloud_cwp_license_order",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"license_num": 3,
		},
	}
	req, err := YunjingLicense{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Product != "yunjing" || req.Action != "InquiryPriceOpenProVersionPrepaid" {
		t.Fatalf("unexpected route: %s/%s", req.Product, req.Action)
	}
	// Always prices a single month.
	cp, ok := req.Params["ChargePrepaid"].(map[string]interface{})
	if !ok || cp["Period"] != 1 {
		t.Fatalf("ChargePrepaid.Period = %v, want 1", cp["Period"])
	}
	machines, ok := req.Params["Machines"].([]interface{})
	if !ok || len(machines) != 3 {
		t.Fatalf("Machines len = %d, want 3", len(machines))
	}
	m0 := machines[0].(map[string]interface{})
	if m0["MachineType"] != "CVM" || m0["MachineRegion"] != "ap-guangzhou" {
		t.Fatalf("machine[0] = %v", m0)
	}
}

func TestYunjingExtractDefaultsToOne(t *testing.T) {
	req, err := YunjingLicense{}.Extract(parser.PlannedResource{
		Type:   "tencentcloud_cwp_license_order",
		Region: "ap-shanghai",
		After:  map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got := len(req.Params["Machines"].([]interface{})); got != 1 {
		t.Fatalf("default Machines = %d, want 1", got)
	}
}

func TestYunjingParsePrefersDiscountAndResponse(t *testing.T) {
	req := pricing.PriceRequest{
		Params: map[string]interface{}{
			"Machines": []interface{}{map[string]interface{}{}, map[string]interface{}{}},
		},
	}
	// Response block present -> preferred over top-level; DiscountPrice preferred.
	raw := []byte(`{"OriginalPrice":100,"DiscountPrice":90,"Response":{"OriginalPrice":60,"DiscountPrice":48}}`)
	comps, err := YunjingLicense{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if !almostEqYunjing(comps[0].MonthlyCost, 48) {
		t.Fatalf("monthly = %v, want 48 (Response.DiscountPrice)", comps[0].MonthlyCost)
	}
	if comps[0].Currency != "CNY" || comps[0].Unit != "MONTH" {
		t.Fatalf("unexpected component meta: %+v", comps[0])
	}
}

func TestYunjingParseFallbackToOriginal(t *testing.T) {
	req := pricing.PriceRequest{Params: map[string]interface{}{"Machines": []interface{}{map[string]interface{}{}}}}
	// No discount -> fall back to original; no Response block -> use top-level.
	raw := []byte(`{"OriginalPrice":75,"DiscountPrice":0}`)
	comps, err := YunjingLicense{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !almostEqYunjing(comps[0].MonthlyCost, 75) {
		t.Fatalf("monthly = %v, want 75", comps[0].MonthlyCost)
	}
}
