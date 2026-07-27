package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// EIP handles `tencentcloud_eip`.
//
// Pricing API (vpc): InquiryPriceAllocateAddresses.
// Docs: https://cloud.tencent.com/document/api/215/114855
//
// The SDK has no typed method for this action; the handler uses CommonRequest
// (see handlers.go "vpc" → "InquiryPriceAllocateAddresses").
//
// Response (CNY):
//   - POSTPAID (TRAFFIC/BANDWIDTH_POSTPAID_BY_HOUR):
//     Price.AddressPrice.{UnitPrice, DiscountPrice, ChargeUnit="HOUR"}
//   - PREPAID (BANDWIDTH_PREPAID_BY_MONTH):
//     Price.AddressPrice.{OriginalPrice, DiscountPrice} (total for the period)
type EIP struct{}

func (EIP) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	chargeType := strings.TrimSpace(getStr(r.After, "internet_charge_type"))
	if chargeType == "" {
		chargeType = "TRAFFIC_POSTPAID_BY_HOUR"
	}

	bw := getInt(r.After, "internet_max_bandwidth_out")
	if bw <= 0 {
		bw = 1
	}

	params := map[string]interface{}{
		"InternetChargeType":      chargeType,
		"InternetMaxBandwidthOut": bw,
	}

	// For prepaid, add AddressChargePrepaid with Period=1.
	if strings.Contains(strings.ToUpper(chargeType), "PREPAID") {
		params["AddressChargePrepaid"] = map[string]interface{}{
			"Period": 1,
		}
	}

	return pricing.PriceRequest{
		Product: "vpc",
		Action:  "InquiryPriceAllocateAddresses",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (EIP) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	type addressPrice struct {
		UnitPrice     float64 `json:"UnitPrice"`
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
		ChargeUnit    string  `json:"ChargeUnit"`
	}
	var wrap struct {
		Price struct {
			AddressPrice addressPrice `json:"AddressPrice"`
		} `json:"Price"`
		Response struct {
			Price struct {
				AddressPrice addressPrice `json:"AddressPrice"`
			} `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	ap := wrap.Price.AddressPrice
	if wrap.Response.Price.AddressPrice.UnitPrice > 0 ||
		wrap.Response.Price.AddressPrice.DiscountPrice > 0 ||
		wrap.Response.Price.AddressPrice.OriginalPrice > 0 {
		ap = wrap.Response.Price.AddressPrice
	}

	chargeType := fmt.Sprintf("%v", req.Params["InternetChargeType"])
	prepaid := strings.Contains(strings.ToUpper(chargeType), "PREPAID")

	var monthly, hourly float64
	if prepaid {
		// Prepaid: DiscountPrice is the total for the period (1 month).
		monthly = ap.DiscountPrice
		if monthly == 0 {
			monthly = ap.OriginalPrice
		}
		hourly = 0
	} else {
		// Postpaid: DiscountPrice is the per-hour rate.
		rate := ap.DiscountPrice
		if rate == 0 {
			rate = ap.UnitPrice
		}
		hourly = rate
		monthly = rate * hoursPerMonth
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("EIP (%v Mbps, %v)", req.Params["InternetMaxBandwidthOut"], chargeType),
		Unit:        ap.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
