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
// Docs: https://cloud.tencent.com/document/api/215/17517
//
// Terraform provider fields commonly seen:
//   - bandwidth (Mbps), charge_type (PREPAID | POSTPAID_BY_HOUR),
//     prepaid_period, type (IPSEC | SSL), max_connection (SSL only)
//
// Response.Price.InstancePrice.{UnitPrice,DiscountPrice,ChargeUnit} is in CNY.
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
		period := getInt(r.After, "prepaid_period")
		if period <= 0 {
			period = getInt(r.After, "period")
		}
		if period <= 0 {
			period = 1
		}
		params["InstanceChargePrepaid"] = map[string]interface{}{
			"Period":    period,
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

func (VPNGateway) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// itemPrice mirrors vpc ItemPrice (CNY). UnitPrice is CNY/hour for POSTPAID.
	type itemPrice struct {
		UnitPrice     float64 `json:"UnitPrice"`
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
		ChargeUnit    string  `json:"ChargeUnit"`
	}
	type priceBlock struct {
		InstancePrice  itemPrice `json:"InstancePrice"`
		BandwidthPrice itemPrice `json:"BandwidthPrice"`
	}
	var wrap struct {
		Price    priceBlock `json:"Price"`
		Response struct {
			Price priceBlock `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	price := wrap.Price
	// The Tencent SDK nests the real payload under "Response"; prefer it when present.
	if wrap.Response.Price.InstancePrice.UnitPrice > 0 ||
		wrap.Response.Price.InstancePrice.DiscountPrice > 0 ||
		wrap.Response.Price.BandwidthPrice.UnitPrice > 0 {
		price = wrap.Response.Price
	}

	comps := make([]output.CostComponent, 0, 2)

	ip := price.InstancePrice
	monthly, hourly := monthlyFromPrice(ip.ChargeUnit, ip.UnitPrice, ip.DiscountPrice)
	comps = append(comps, output.CostComponent{
		Name:        fmt.Sprintf("VPN gateway (%v Mbps)", req.Params["InternetMaxBandwidthOut"]),
		Unit:        ip.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	})

	if bw := price.BandwidthPrice; bw.UnitPrice > 0 || bw.DiscountPrice > 0 {
		bwMonthly, bwHourly := monthlyFromPrice(bw.ChargeUnit, bw.UnitPrice, bw.DiscountPrice)
		comps = append(comps, output.CostComponent{
			Name:        "VPN public bandwidth",
			Unit:        bw.ChargeUnit,
			HourlyCost:  bwHourly,
			MonthlyCost: bwMonthly,
			Currency:    "CNY",
		})
	}

	return comps, nil
}
