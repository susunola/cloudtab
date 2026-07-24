package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// YunjingLicense handles `tencentcloud_cwp_license_order` (Cloud Workload
// Protection / Host Security professional-version license).
//
// Pricing API (yunjing): InquiryPriceOpenProVersionPrepaid (Inquiry, with 'y').
// Docs: https://cloud.tencent.com/document/api/296/19836
//
// The professional version is a prepaid, per-machine license. The request takes
// a ChargePrepaid{Period,RenewFlag} block plus a Machines list; cloudtab always
// prices a single month (Period=1) so the returned total is the monthly
// run-rate. Response.{OriginalPrice,DiscountPrice} are float64 in CNY for the
// whole requested period (no per-instance currency field; CNY is implied).
type YunjingLicense struct{}

func (YunjingLicense) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	// license_num drives how many machine licenses are priced. Fall back to 1.
	count := getInt(r.After, "license_num")
	if count <= 0 {
		count = 1
	}

	// The Machines list must be non-empty for the API. We do not have concrete
	// machine identifiers in a plan, so we synthesise `count` CVM entries in the
	// resource region — price scales linearly with the number of machines.
	region := strings.TrimSpace(r.Region)
	machines := make([]interface{}, 0, count)
	for i := int64(0); i < count; i++ {
		m := map[string]interface{}{"MachineType": "CVM"}
		if region != "" {
			m["MachineRegion"] = region
		}
		machines = append(machines, m)
	}

	params := map[string]interface{}{
		// Always price a single month; cloudtab reports a monthly run-rate.
		"ChargePrepaid": map[string]interface{}{
			"Period":    1,
			"RenewFlag": "NOTIFY_AND_AUTO_RENEW",
		},
		"Machines": machines,
	}

	return pricing.PriceRequest{
		Product: "yunjing",
		Action:  "InquiryPriceOpenProVersionPrepaid",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (YunjingLicense) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	type priceBlock struct {
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
	}
	var wrap struct {
		priceBlock
		Response struct {
			priceBlock
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	pb := wrap.priceBlock
	if wrap.Response.OriginalPrice > 0 || wrap.Response.DiscountPrice > 0 {
		pb = wrap.Response.priceBlock
	}

	// Prefer the discounted price; fall back to the original (already CNY/month).
	monthly := preferDiscount(pb.DiscountPrice, pb.OriginalPrice)

	n := len(req.Params["Machines"].([]interface{}))
	return []output.CostComponent{{
		Name:        fmt.Sprintf("CWP Pro license (x%d)", n),
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
