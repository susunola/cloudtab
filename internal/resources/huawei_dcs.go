package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiDCS handles `huaweicloud_dcs_instance` (DCS).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiDCS struct{}

func (HuaweiDCS) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	flavor := strings.TrimSpace(getStr(r.After, "flavor"))
	if flavor == "" {
		flavor = "redis.single.xu1.large.1"
	}
	// Default engine is "Redis" when absent from plan.
	_ = getStr(r.After, "engine")

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "dcs",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": []map[string]interface{}{
				huaweiProductInfo("hws.service.type.dcs", "hws.resource.type.dcs", flavor, r.Region, 0, 0),
			},
		},
	}, nil
}

func (HuaweiDCS) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei DCS",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
