package resources

import (
	"encoding/json"
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// CLBInstance handles `tencentcloud_clb_instance`.
//
// Reference: https://cloud.tencent.com/document/api/214/41708
// (InquiryPriceCreateLoadBalancer → Response.Price)
//
// CLB pricing has two dimensions:
//   - Instance fee (LB itself, per hour or fixed monthly for prepaid)
//   - Traffic/bandwidth fee (depends on internet_charge_type)
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

// clbPriceBlock is the nested price structure returned by the CLB
// InquiryPriceCreateLoadBalancer API.
type clbPriceBlock struct {
	InstancePrice struct {
		UnitPrice         float64 `json:"UnitPrice"`
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		OriginalPrice     float64 `json:"OriginalPrice"`
		DiscountPrice     float64 `json:"DiscountPrice"`
		ChargeUnit        string  `json:"ChargeUnit"`
		Currency          string  `json:"Currency"`
	} `json:"InstancePrice"`
	BandwidthPrice struct {
		UnitPrice         float64 `json:"UnitPrice"`
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		ChargeUnit        string  `json:"ChargeUnit"`
	} `json:"BandwidthPrice"`
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
	if wrap.Response.Price.InstancePrice.UnitPriceDiscount > 0 ||
		wrap.Response.Price.InstancePrice.DiscountPrice > 0 ||
		wrap.Response.Price.InstancePrice.UnitPrice > 0 {
		price = wrap.Response.Price
	}

	currency := price.InstancePrice.Currency
	if currency == "" {
		currency = "CNY"
	}

	ip := price.InstancePrice
	monthly, hourly := monthlyFromPrice(ip.ChargeUnit, ip.UnitPriceDiscount, ip.DiscountPrice)
	label := fmt.Sprintf("CLB (%v)", req.Params["LoadBalancerType"])
	comps := []output.CostComponent{{
		Name:        label,
		Unit:        ip.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    currency,
	}}
	if price.BandwidthPrice.UnitPrice > 0 {
		bw := price.BandwidthPrice
		bwMonthly, bwHourly := monthlyFromPrice(bw.ChargeUnit, bw.UnitPriceDiscount, 0)
		comps = append(comps, output.CostComponent{
			Name:        "CLB bandwidth",
			Unit:        bw.ChargeUnit,
			HourlyCost:  bwHourly,
			MonthlyCost: bwMonthly,
			Currency:    currency,
		})
	}
	return comps, nil
}
