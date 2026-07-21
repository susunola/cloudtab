package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// GAAPProxy handles `tencentcloud_gaap_proxy` (Global Application Acceleration).
//
// Pricing API (gaap): InquiryPriceCreateProxy.
// Docs: https://cloud.tencent.com/document/api/608/37569
//
// GAAP quotes a DAILY proxy price. Response.{ProxyDailyPrice,
// DiscountProxyDailyPrice} are float64 in 元/day. We convert to a monthly
// run-rate with the shared 730h month (≈30.4 days). BillingType is an int:
// 0 = by bandwidth, 1 = by flow.
type GAAPProxy struct{}

// daysPerMonth mirrors hoursPerMonth (730h) expressed in days, so a daily
// GAAP rate lines up with the rest of cloudtab's monthly convention.
const daysPerMonth = hoursPerMonth / 24.0

func (GAAPProxy) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	accessRegion := strings.TrimSpace(getStr(r.After, "access_region"))
	realServerRegion := strings.TrimSpace(getStr(r.After, "realserver_region"))
	if realServerRegion == "" {
		realServerRegion = strings.TrimSpace(getStr(r.After, "real_server_region"))
	}
	bandwidth := getInt(r.After, "bandwidth")
	// The Terraform tencentcloud_gaap_proxy schema already stores `concurrent`
	// in units of 万 (10k connections), which is exactly what the API expects,
	// so we pass it through unchanged — no scaling heuristics.
	concurrent := getInt(r.After, "concurrent")
	if accessRegion == "" || realServerRegion == "" || bandwidth <= 0 || concurrent <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_gaap_proxy requires access_region/realserver_region/bandwidth/concurrent")
	}

	// billing_type: 0 = by bandwidth (default), 1 = by flow. Accept either a
	// numeric value (1) or a string ("1"/"flow") from the plan.
	billingType := 0
	if getInt(r.After, "billing_type") == 1 {
		billingType = 1
	} else if bt := strings.ToLower(strings.TrimSpace(getStr(r.After, "billing_type"))); bt == "1" || bt == "flow" {
		billingType = 1
	}

	params := map[string]interface{}{
		"AccessRegion":     accessRegion,
		"RealServerRegion": realServerRegion,
		"Bandwidth":        bandwidth,
		"Concurrent":       concurrent,
		"BillingType":      billingType,
	}
	if dr := strings.TrimSpace(getStr(r.After, "dest_region")); dr != "" {
		params["DestRegion"] = dr
	}

	return pricing.PriceRequest{
		Product: "gaap",
		Action:  "InquiryPriceCreateProxy",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (GAAPProxy) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		ProxyDailyPrice         float64 `json:"ProxyDailyPrice"`
		DiscountProxyDailyPrice float64 `json:"DiscountProxyDailyPrice"`
		Response                struct {
			ProxyDailyPrice         float64 `json:"ProxyDailyPrice"`
			DiscountProxyDailyPrice float64 `json:"DiscountProxyDailyPrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	daily := wrap.DiscountProxyDailyPrice
	orig := wrap.ProxyDailyPrice
	if wrap.Response.DiscountProxyDailyPrice > 0 || wrap.Response.ProxyDailyPrice > 0 {
		daily = wrap.Response.DiscountProxyDailyPrice
		orig = wrap.Response.ProxyDailyPrice
	}
	// Prefer the discounted daily price; fall back to the original.
	daily = preferDiscount(daily, orig)

	return []output.CostComponent{{
		Name:        fmt.Sprintf("GAAP proxy (%v Mbps)", req.Params["Bandwidth"]),
		Unit:        "DAY",
		HourlyCost:  0,
		MonthlyCost: daily * daysPerMonth,
		Currency:    "CNY",
	}}, nil
}
