package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaEIP handles `alicloud_eip` (elastic IP).
//
// Priced via Alibaba Cloud BSS GetPayAsYouGoPrice with ProductCode "eip".
// ModuleList: Bandwidth (Kbps, per-DAY), InternetChargeType (0 = by-bandwidth,
// 1 = by-traffic), ISP (BGP). EIP bandwidth is quoted per day, so the monthly
// figure uses daysPerMonth (30), not hoursPerMonth.
type AlibabaEIP struct{}

func (AlibabaEIP) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	// Terraform bandwidth is in Mbps; the BSS API expects Kbps (1024..204800).
	bwKbps := getInt(r.After, "bandwidth") * 1024
	if bwKbps <= 0 {
		bwKbps = 5 * 1024
	}

	// 0 = pay-by-bandwidth, 1 = pay-by-traffic.
	chargeType := 0
	if strings.EqualFold(strings.TrimSpace(getStr(r.After, "internet_charge_type")), "PayByTraffic") {
		chargeType = 1
	}

	moduleList := []map[string]string{
		alibabaModule("Bandwidth", "Day", fmt.Sprintf("Bandwidth:%d", bwKbps)),
		alibabaModule("InternetChargeType", "Usage", fmt.Sprintf("InternetChargeType:%d", chargeType)),
		alibabaModule("ISP", "Hour", "ISP:BGP"),
	}

	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "eip",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList":       moduleList,
		},
	}, nil
}

func (AlibabaEIP) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	modules, currency, err := parseAlibabaModules(raw)
	if err != nil {
		return nil, err
	}

	// EIP mixes modules with different billing units:
	//   - Bandwidth is priced per DAY.
	//   - ISP is priced per HOUR.
	//   - InternetChargeType is a Usage classifier with no standalone cost.
	// Convert each module with its actual unit rather than folding everything
	// into a single daily rate.
	var comps []output.CostComponent
	for _, m := range modules {
		var name, unit string
		var hourly, monthly float64
		switch m.ModuleCode {
		case "Bandwidth":
			name = "Alibaba EIP bandwidth"
			unit = "DAY"
			hourly = m.CostYuan / daysPerMonth
			monthly = m.CostYuan * daysPerMonth
		case "ISP":
			name = "Alibaba EIP ISP"
			unit = "HOUR"
			hourly = m.CostYuan
			monthly = m.CostYuan * hoursPerMonth
		default:
			// Usage classifiers (InternetChargeType) have no cost of their own.
			continue
		}
		comps = append(comps, output.CostComponent{
			Name:        name,
			Unit:        unit,
			HourlyCost:  hourly,
			MonthlyCost: monthly,
			Currency:    currency,
		})
	}
	if len(comps) == 0 {
		// Fallback for responses that do not include ModuleCode: report a single
		// daily component so callers still see a total.
		info, _ := parseAlibabaPrice(raw)
		comps = append(comps, output.CostComponent{
			Name:        "Alibaba EIP",
			Unit:        "DAY",
			HourlyCost:  info.PriceYuan / daysPerMonth,
			MonthlyCost: info.PriceYuan * daysPerMonth,
			Currency:    info.Currency,
		})
	}
	return comps, nil
}
