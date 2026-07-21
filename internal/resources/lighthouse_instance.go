package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// LighthouseInstance handles `tencentcloud_lighthouse_instance`.
//
// Pricing API (lighthouse): InquirePriceCreateInstances (SDK spells it
// "InquirePrice", without the 'y').
// Docs: https://cloud.tencent.com/document/api/1207/47576
//
// Lighthouse is a prepaid, bundle-priced product: you buy a BundleId for a
// number of months. Response.Price.InstancePrice.{OriginalPrice,DiscountPrice}
// is in 元 and is the total for the requested Period. cloudtab always prices a
// single month (Period=1) so the returned price is the monthly run-rate.
type LighthouseInstance struct{}

func (LighthouseInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	bundleID := strings.TrimSpace(getStr(r.After, "bundle_id"))
	if bundleID == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_lighthouse_instance requires bundle_id")
	}

	count := getInt(r.After, "instance_count")
	if count <= 0 {
		count = 1
	}

	params := map[string]interface{}{
		"BundleId":      bundleID,
		"InstanceCount": count,
		// Always price a single month; cloudtab reports a monthly run-rate.
		"InstanceChargePrepaid": map[string]interface{}{
			"Period":    1,
			"RenewFlag": "NOTIFY_AND_AUTO_RENEW",
		},
	}
	if bp := strings.TrimSpace(getStr(r.After, "blueprint_id")); bp != "" {
		params["BlueprintId"] = bp
	}

	return pricing.PriceRequest{
		Product: "lighthouse",
		Action:  "InquirePriceCreateInstances",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (LighthouseInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	type instancePrice struct {
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
	}
	type priceBlock struct {
		InstancePrice instancePrice `json:"InstancePrice"`
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

	ip := wrap.Price.InstancePrice
	if wrap.Response.Price.InstancePrice.OriginalPrice > 0 ||
		wrap.Response.Price.InstancePrice.DiscountPrice > 0 {
		ip = wrap.Response.Price.InstancePrice
	}

	// Prefer the discounted price; fall back to the original (already 元/month).
	monthly := preferDiscount(ip.DiscountPrice, ip.OriginalPrice)

	return []output.CostComponent{{
		Name:        fmt.Sprintf("Lighthouse (%v)", req.Params["BundleId"]),
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
