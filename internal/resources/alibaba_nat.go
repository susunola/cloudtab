package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaNAT handles `alicloud_nat_gateway`.
//
// Priced via Alibaba Cloud BSS GetPayAsYouGoPrice with ProductCode "natgateway".
// ModuleList: Specification.
type AlibabaNAT struct{}

func (AlibabaNAT) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	spec := strings.TrimSpace(getStr(r.After, "specification"))
	if spec == "" {
		spec = "Small"
	}

	// Extracted for reference; not used in the price request.
	_ = strings.TrimSpace(getStr(r.After, "vswitch_id"))

	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "natgateway",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList": []map[string]string{
				alibabaModule("Specification", "Hour", spec),
			},
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
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
