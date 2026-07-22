package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

func almostEqHSM(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCloudHSMExtract(t *testing.T) {
	req, err := CloudHSMInstance{}.Extract(parser.PlannedResource{
		Type:   "tencentcloud_cloudhsm_instance",
		Region: "ap-guangzhou",
		After:  map[string]interface{}{"goods_num": 2},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Product != "cloudhsm" || req.Action != "InquiryPriceBuyVsm" {
		t.Fatalf("unexpected route: %s/%s", req.Product, req.Action)
	}
	if req.Params["GoodsNum"] != int64(2) {
		t.Fatalf("GoodsNum = %v, want 2", req.Params["GoodsNum"])
	}
	// Always prices a single month, prepaid.
	if req.Params["PayMode"] != 1 || req.Params["TimeSpan"] != "1" || req.Params["TimeUnit"] != "m" {
		t.Fatalf("expected single-month prepaid, got %+v", req.Params)
	}
}

func TestCloudHSMParsePrefersTotalCostAndResponse(t *testing.T) {
	req := pricing.PriceRequest{Params: map[string]interface{}{"GoodsNum": int64(1)}}
	// Response block present; TotalCost (payable) preferred over OriginalCost.
	raw := []byte(`{"TotalCost":999,"OriginalCost":999,"Response":{"TotalCost":88.5,"OriginalCost":120}}`)
	comps, err := CloudHSMInstance{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !almostEqHSM(comps[0].MonthlyCost, 88.5) {
		t.Fatalf("monthly = %v, want 88.5", comps[0].MonthlyCost)
	}
}

func TestCloudHSMParseFallbackToOriginal(t *testing.T) {
	req := pricing.PriceRequest{Params: map[string]interface{}{"GoodsNum": int64(1)}}
	// TotalCost zero -> fall back to OriginalCost.
	raw := []byte(`{"Response":{"TotalCost":0,"OriginalCost":150}}`)
	comps, err := CloudHSMInstance{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !almostEqHSM(comps[0].MonthlyCost, 150) {
		t.Fatalf("monthly = %v, want 150", comps[0].MonthlyCost)
	}
}
