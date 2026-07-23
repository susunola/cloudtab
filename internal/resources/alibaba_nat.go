package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaNAT handles `alicloud_nat_gateway`.
//
// Priced via Alibaba Cloud BSS GetPayAsYouGoPrice with ProductCode "nat_gw"
// (NOT "natgateway"). ModuleList: Spec (Small/Middle/Large/Xlarge.1), quoted
// per DAY, so the monthly figure uses daysPerMonth (30).
type AlibabaNAT struct{}

func (AlibabaNAT) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	spec := strings.TrimSpace(getStr(r.After, "specification"))
	if spec == "" {
		spec = "Small"
	}

	moduleList := []map[string]string{
		alibabaModule("Spec", "Day", "Spec:"+spec),
	}

	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "nat_gw",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList":       moduleList,
		},
	}, nil
}

func (AlibabaNAT) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba NAT Gateway",
		Unit:        "DAY",
		HourlyCost:  info.PriceYuan / daysPerMonth,
		MonthlyCost: info.PriceYuan * daysPerMonth,
		Currency:    info.Currency,
	}}, nil
}
