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

	params := map[string]interface{}{
		"LoadBalancerType": lbType,
		"GoodsNum":         1,
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

func (CLBInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		Price struct {
			InstancePrice struct {
				UnitPrice         float64 `json:"UnitPrice"`
				UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
				OriginalPrice     float64 `json:"OriginalPrice"`
				DiscountPrice     float64 `json:"DiscountPrice"`
				ChargeUnit        string  `json:"ChargeUnit"`
			} `json:"InstancePrice"`
			BandwidthPrice struct {
				UnitPrice         float64 `json:"UnitPrice"`
				UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
				ChargeUnit        string  `json:"ChargeUnit"`
			} `json:"BandwidthPrice"`
		} `json:"Price"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	ip := wrap.Price.InstancePrice
	monthly, hourly := monthlyFromPrice(ip.ChargeUnit, ip.UnitPriceDiscount, ip.DiscountPrice)
	label := fmt.Sprintf("CLB (%v)", req.Params["LoadBalancerType"])
	comps := []output.CostComponent{{
		Name:        label,
		Unit:        ip.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}
	if wrap.Price.BandwidthPrice.UnitPrice > 0 {
		bw := wrap.Price.BandwidthPrice
		bwMonthly, bwHourly := monthlyFromPrice(bw.ChargeUnit, bw.UnitPriceDiscount, 0)
		comps = append(comps, output.CostComponent{
			Name:        "CLB bandwidth",
			Unit:        bw.ChargeUnit,
			HourlyCost:  bwHourly,
			MonthlyCost: bwMonthly,
			Currency:    "CNY",
		})
	}
	return comps, nil
}
