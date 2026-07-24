package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiNAT handles `huaweicloud_nat_gateway` (NAT Gateway).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiNAT struct{}

func (HuaweiNAT) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	spec := strings.TrimSpace(getStr(r.After, "spec"))
	if spec == "" {
		spec = "1"
	}

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "nat",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": []map[string]interface{}{
				huaweiProductInfo("hws.service.type.nat", "hws.resource.type.natgateway", spec, r.Region, 0, 0),
			},
		},
	}, nil
}

func (HuaweiNAT) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei NAT Gateway",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
