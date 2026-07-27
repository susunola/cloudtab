package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// CLBInstance handles `tencentcloud_clb_instance`.
//
// Reference: https://cloud.tencent.com/document/product/214/98697
// (InquiryPriceCreateLoadBalancer → Response.Price)
//
// CLB pricing has up to three dimensions (see Price data structure:
// https://cloud.tencent.com/document/api/214/30694#Price); any may be null:
//   - Instance fee (LB itself, per hour or fixed monthly for prepaid)
//   - Bandwidth/traffic fee (ChargeUnit "HOUR" = 元/h, "GB" = 元/GB by traffic)
//   - LCU fee (LCU-style performance-guaranteed instances only, 元/h)
type CLBInstance struct{}

func (CLBInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	lbType := getStr(r.After, "network_type") // OPEN | INTERNAL
	if lbType == "" {
		lbType = "OPEN"
	}
	forward := getStr(r.After, "clb_type") // "PERFORMANCE" | "SHARED" — legacy compat
	slaType := getStr(r.After, "sla_type") // clb.c2.medium etc for LCU-CLB
	chargeType := getStr(r.After, "internet_charge_type")
	if chargeType == "" {
		chargeType = "TRAFFIC_POSTPAID_BY_HOUR"
	}

	// LoadBalancerChargeType is required by the InquiryPriceCreateLoadBalancer
	// API. The Terraform provider does not expose a dedicated field for this;
	// the tencentcloud_clb_instance resource is always pay-as-you-go, so we
	// default to "POSTPAID".
	lbChargeType := getStr(r.After, "load_balancer_charge_type")
	if lbChargeType == "" {
		lbChargeType = "POSTPAID"
	}

	params := map[string]interface{}{
		"LoadBalancerType":       lbType,
		"LoadBalancerChargeType": lbChargeType,
		"GoodsNum":               1,
	}
	if forward != "" {
		params["Forward"] = forward
	}
	if slaType != "" {
		params["SlaType"] = slaType
	}
	if bw := getInt(r.After, "internet_max_bandwidth_out"); bw > 0 {
		params["InternetAccessible"] = map[string]interface{}{
			"InternetChargeType":      chargeType,
			"InternetMaxBandwidthOut": bw,
		}
	}

	return pricing.PriceRequest{
		Product: "clb",
		Action:  "InquiryPriceCreateLoadBalancer",
		Region:  r.Region,
		Params:  params,
	}, nil
}

// clbItemPrice mirrors the CLB ItemPrice data structure
// (https://cloud.tencent.com/document/api/214/30694#ItemPrice).
// Discount is informational only (e.g. 20.0 = 2折) and not used for arithmetic.
type clbItemPrice struct {
	UnitPrice         float64 `json:"UnitPrice"`
	UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
	OriginalPrice     float64 `json:"OriginalPrice"`
	DiscountPrice     float64 `json:"DiscountPrice"`
	ChargeUnit        string  `json:"ChargeUnit"`
	Discount          float64 `json:"Discount"`
}

// clbPriceBlock mirrors the CLB Price data structure returned by
// InquiryPriceCreateLoadBalancer
// (https://cloud.tencent.com/document/api/214/30694#Price). BandwidthPrice and
// LcuPrice may be null depending on the CLB type and billing mode.
type clbPriceBlock struct {
	InstancePrice  clbItemPrice `json:"InstancePrice"`
	BandwidthPrice clbItemPrice `json:"BandwidthPrice"`
	LcuPrice       clbItemPrice `json:"LcuPrice"`
}

func (CLBInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// The Tencent Cloud SDK wraps real responses under a "Response" key.
	// Support both the wrapped format (real API) and the unwrapped format
	// (test mocks) for robustness.
	var wrap struct {
		Price    clbPriceBlock `json:"Price"`
		Response struct {
			Price clbPriceBlock `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	// Prefer the Response-wrapped price when it carries data.
	price := wrap.Price
	if hasClbPriceData(wrap.Response.Price) {
		price = wrap.Response.Price
	}

	// ItemPrice carries no Currency field (see docs); CLB is always CNY.
	const currency = "CNY"

	var comps []output.CostComponent
	// Instance fee (always present).
	comps = append(comps, clbComponent(
		fmt.Sprintf("CLB (%v)", req.Params["LoadBalancerType"]),
		price.InstancePrice, currency))
	// Bandwidth / traffic fee (may be null).
	if hasClbItemPrice(price.BandwidthPrice) {
		comps = append(comps, clbComponent("CLB bandwidth", price.BandwidthPrice, currency))
	}
	// LCU fee (only for LCU-style performance-guaranteed instances; may be null).
	if hasClbItemPrice(price.LcuPrice) {
		comps = append(comps, clbComponent("CLB LCU", price.LcuPrice, currency))
	}
	return comps, nil
}

// hasClbItemPrice reports whether an ItemPrice carries any non-zero data.
func hasClbItemPrice(p clbItemPrice) bool {
	return p.UnitPrice > 0 || p.UnitPriceDiscount > 0 ||
		p.DiscountPrice > 0 || p.OriginalPrice > 0
}

// hasClbPriceData reports whether any dimension of a Price block has data.
func hasClbPriceData(p clbPriceBlock) bool {
	return hasClbItemPrice(p.InstancePrice) ||
		hasClbItemPrice(p.BandwidthPrice) ||
		hasClbItemPrice(p.LcuPrice)
}

// clbComponent builds a CostComponent from a CLB ItemPrice. Per the official
// ItemPrice definition ChargeUnit is either "HOUR" (元/hour) or "GB"
// (元/GB, traffic-based). For "GB" we cannot estimate a monthly cost without
// traffic volume, so only the rate is shown; for "HOUR" (and PREPAID totals
// where ChargeUnit is empty) monthlyFromPrice does the conversion.
func clbComponent(name string, p clbItemPrice, currency string) output.CostComponent {
	unit := strings.ToUpper(strings.TrimSpace(p.ChargeUnit))
	if unit == "GB" {
		// Per-traffic billing: UnitPrice/UnitPriceDiscount is 元/GB.
		rate := preferDiscount(p.UnitPriceDiscount, p.UnitPrice)
		return output.CostComponent{
			Name:        fmt.Sprintf("%s (%.4f 元/GB)", name, rate),
			Unit:        "GB",
			HourlyCost:  0,
			MonthlyCost: 0,
			Currency:    currency,
		}
	}
	unitPrice := preferDiscount(p.UnitPriceDiscount, p.UnitPrice)
	monthly, hourly := monthlyFromPrice(p.ChargeUnit, unitPrice, p.DiscountPrice)
	return output.CostComponent{
		Name:        name,
		Unit:        p.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    currency,
	}
}
