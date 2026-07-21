package resources

import (
	"encoding/json"
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// EIP handles `tencentcloud_eip`.
//
// Reference: https://cloud.tencent.com/document/api/215/16699
// (InquiryPriceCreateAddresses → Response.AddressPrice)
//
// Cost model:
//   - BANDWIDTH_POSTPAID_BY_HOUR: per-hour bandwidth fee (Mbps × time)
//   - TRAFFIC_POSTPAID_BY_HOUR:   per-GB traffic fee — requires usage.yml
//     (monthly_gb) because the plan has no traffic volume
//   - BANDWIDTH_PACKAGE:          fixed monthly fee via BandwidthPackage
type EIP struct{}

func (EIP) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	getStr := func(k string) string {
		if v, ok := r.After[k].(string); ok {
			return v
		}
		return ""
	}
	getInt := func(k string) int64 {
		switch v := r.After[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		}
		return 0
	}

	chargeType := getStr("internet_charge_type")
	if chargeType == "" {
		chargeType = "BANDWIDTH_POSTPAID_BY_HOUR"
	}
	bw := getInt("internet_max_bandwidth_out")
	if bw == 0 {
		bw = 1 // default 1Mbps if unspecified in plan
	}

	params := map[string]interface{}{
		"InternetChargeType":     chargeType,
		"InternetMaxBandwidthOut": bw,
		"AddressCount":           1,
	}

	return pricing.PriceRequest{
		Product: "vpc",
		Action:  "InquiryPriceCreateAddresses",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (EIP) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		AddressPrice struct {
			UnitPrice         float64 `json:"UnitPrice"`
			UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
			OriginalPrice     float64 `json:"OriginalPrice"`
			DiscountPrice     float64 `json:"DiscountPrice"`
			ChargeUnit        string  `json:"ChargeUnit"`
		} `json:"AddressPrice"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	ap := wrap.AddressPrice
	monthly := ap.UnitPriceDiscount * 730
	if ap.OriginalPrice > 0 {
		monthly = ap.DiscountPrice
	}
	return []output.CostComponent{{
		Name:        fmt.Sprintf("EIP (%dMbps, %s)", req.Params["InternetMaxBandwidthOut"], req.Params["InternetChargeType"]),
		Unit:        ap.ChargeUnit,
		HourlyCost:  ap.UnitPriceDiscount,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
