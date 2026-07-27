package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// VPNGateway handles `tencentcloud_vpn_gateway`.
//
// Pricing API (vpc): InquiryPriceCreateVpnGateway.
// Docs: https://cloud.tencent.com/document/product/215/17512
//
// The API supports both PREPAID (prepaid) and POSTPAID_BY_HOUR (pay-as-you-go).
// cloudtab reports a monthly run-rate, so PREPAID is always priced for a
// single month (Period=1).
//
// Response.Price has two dimensions, each an ItemPrice:
//   - InstancePrice: VPN gateway instance fee
//   - POSTPAID_BY_HOUR: ChargeUnit="HOUR", UnitPrice/DiscountPrice = CNY/hour
//   - PREPAID:          ChargeUnit="" (empty, not "none" as docs suggest),
//     DiscountPrice = period total CNY (Period=1 → monthly)
//   - BandwidthPrice: public network fee
//   - POSTPAID_BY_HOUR: ChargeUnit="GB", UnitPrice/DiscountPrice = CNY/GB (by traffic)
//   - PREPAID:          ChargeUnit="", all zero (included in instance)
//
// For "GB" (per-traffic) billing we cannot estimate a monthly cost without
// traffic volume, so only the rate is shown.
//
// Terraform provider fields commonly seen:
//   - bandwidth (Mbps), charge_type (PREPAID | POSTPAID_BY_HOUR),
//     prepaid_period, type (IPSEC | SSL), max_connection (SSL only)
type VPNGateway struct{}

func (VPNGateway) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	bandwidth := getInt(r.After, "bandwidth")
	if bandwidth <= 0 {
		bandwidth = getInt(r.After, "internet_max_bandwidth_out")
	}
	if bandwidth <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_vpn_gateway requires bandwidth")
	}

	chargeType := strings.ToUpper(strings.TrimSpace(getStr(r.After, "charge_type")))
	if chargeType == "" {
		chargeType = strings.ToUpper(strings.TrimSpace(getStr(r.After, "instance_charge_type")))
	}
	if chargeType == "" {
		chargeType = "POSTPAID_BY_HOUR"
	}

	params := map[string]interface{}{
		"InternetMaxBandwidthOut": bandwidth,
		"InstanceChargeType":      chargeType,
	}

	// SSL gateways price on connection count.
	vpnType := strings.ToUpper(strings.TrimSpace(getStr(r.After, "type")))
	if vpnType != "" {
		params["Type"] = vpnType
	}
	if conn := getInt(r.After, "max_connection"); conn > 0 {
		params["MaxConnection"] = conn
	}

	if chargeType == "PREPAID" {
		params["InstanceChargePrepaid"] = map[string]interface{}{
			// Always price a single month: cloudtab reports a monthly run-rate
			// and the PREPAID DiscountPrice is a period total, so Period=1
			// keeps it monthly.
			"Period":    1,
			"RenewFlag": "NOTIFY_AND_AUTO_RENEW",
		}
	}

	return pricing.PriceRequest{
		Product: "vpc",
		Action:  "InquiryPriceCreateVpnGateway",
		Region:  r.Region,
		Params:  params,
	}, nil
}

// vpnItemPrice mirrors the vpc ItemPrice data structure
// (https://cloud.tencent.com/document/api/215/15824#ItemPrice).
type vpnItemPrice struct {
	UnitPrice         float64 `json:"UnitPrice"`
	UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
	OriginalPrice     float64 `json:"OriginalPrice"`
	DiscountPrice     float64 `json:"DiscountPrice"`
	ChargeUnit        string  `json:"ChargeUnit"`
}

// vpnPriceBlock is the nested Price structure returned by
// InquiryPriceCreateVpnGateway, carrying instance and bandwidth dimensions.
type vpnPriceBlock struct {
	InstancePrice  vpnItemPrice `json:"InstancePrice"`
	BandwidthPrice vpnItemPrice `json:"BandwidthPrice"`
}

func (VPNGateway) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		Price    vpnPriceBlock `json:"Price"`
		Response struct {
			Price vpnPriceBlock `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	// The Tencent SDK nests the real payload under "Response"; prefer it when present.
	price := wrap.Price
	if wrap.Response.Price.InstancePrice.UnitPrice > 0 ||
		wrap.Response.Price.InstancePrice.DiscountPrice > 0 ||
		wrap.Response.Price.BandwidthPrice.UnitPrice > 0 {
		price = wrap.Response.Price
	}

	const currency = "CNY"
	var comps []output.CostComponent

	// Instance fee (always present).
	comps = append(comps, vpnComponent(
		fmt.Sprintf("VPN gateway (%v Mbps)", req.Params["InternetMaxBandwidthOut"]),
		price.InstancePrice, currency))
	// Bandwidth / traffic fee (may be 0 for PREPAID where bandwidth is included).
	if bw := price.BandwidthPrice; bw.UnitPrice > 0 || bw.DiscountPrice > 0 || bw.OriginalPrice > 0 {
		comps = append(comps, vpnComponent("VPN public bandwidth", bw, currency))
	}
	return comps, nil
}

// vpnComponent builds a CostComponent from a VPN ItemPrice. Per the official
// ItemPrice ChargeUnit semantics:
//   - "HOUR" (POSTPAID instance): DiscountPrice is the discounted CNY/hour rate
//     (UnitPrice is the undiscounted rate; UnitPriceDiscount is not returned
//     by this API). Fall back to UnitPrice when DiscountPrice is 0.
//   - "GB"   (POSTPAID bandwidth): CNY/GB, traffic-based — cannot estimate a
//     monthly cost without traffic volume, so only the rate is shown.
//   - "" (PREPAID, empty): DiscountPrice is the period (Period=1 → monthly) total;
//     UnitPrice is 0, so monthlyFromPrice's default branch returns discountPrice.
func vpnComponent(name string, p vpnItemPrice, currency string) output.CostComponent {
	unit := strings.ToUpper(strings.TrimSpace(p.ChargeUnit))
	if unit == "GB" {
		// Per-traffic billing: UnitPrice/DiscountPrice is CNY/GB.
		rate := p.DiscountPrice
		if rate == 0 {
			rate = p.UnitPrice
		}
		return output.CostComponent{
			Name:        fmt.Sprintf("%s (%.4f CNY/GB)", name, rate),
			Unit:        "GB",
			HourlyCost:  0,
			MonthlyCost: 0,
			Currency:    currency,
		}
	}
	if unit == "HOUR" {
		// POSTPAID hourly: DiscountPrice is the discounted CNY/hour rate
		// (preferred over UnitPrice, the undiscounted rate).
		hourly := p.DiscountPrice
		if hourly == 0 {
			hourly = preferDiscount(p.UnitPriceDiscount, p.UnitPrice)
		}
		return output.CostComponent{
			Name:        name,
			Unit:        p.ChargeUnit,
			HourlyCost:  hourly,
			MonthlyCost: hourly * hoursPerMonth,
			Currency:    currency,
		}
	}
	// PREPAID ("", "none", "month"): DiscountPrice is the period (Period=1 →
	// monthly) total; UnitPrice is 0. monthlyFromPrice returns it as-is.
	monthly, hourly := monthlyFromPrice(p.ChargeUnit, 0, p.DiscountPrice)
	return output.CostComponent{
		Name:        name,
		Unit:        p.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    currency,
	}
}
