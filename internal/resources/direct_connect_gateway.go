package resources

import (
	"encoding/json"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// DirectConnectGateway handles `tencentcloud_direct_connect_gateway`.
//
// Pricing API (vpc): InquirePriceCreateDirectConnectGateway.
// Docs: https://cloud.tencent.com/document/api/215/50367
//
// The API takes no business parameters — the price is a flat monthly fee.
// Response.{TotalCost, RealTotalCost} are int64 in 元 (always PREPAID monthly).
type DirectConnectGateway struct{}

func (DirectConnectGateway) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{
		Product: "vpc",
		Action:  "InquirePriceCreateDirectConnectGateway",
		Region:  r.Region,
		Params:  map[string]interface{}{},
	}, nil
}

func (DirectConnectGateway) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		TotalCost     float64 `json:"TotalCost"`
		RealTotalCost float64 `json:"RealTotalCost"`
		Response      struct {
			TotalCost     float64 `json:"TotalCost"`
			RealTotalCost float64 `json:"RealTotalCost"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	cost := wrap.RealTotalCost
	if cost == 0 {
		cost = wrap.Response.RealTotalCost
	}
	if cost == 0 {
		cost = wrap.TotalCost
	}
	if cost == 0 {
		cost = wrap.Response.TotalCost
	}

	return []output.CostComponent{{
		Name:        "Direct Connect Gateway",
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: cost,
		Currency:    "CNY",
	}}, nil
}
