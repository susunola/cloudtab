package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiDDS handles `huaweicloud_dds_instance` (DDS).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiDDS struct{}

func (HuaweiDDS) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	flavor := strings.TrimSpace(getStr(r.After, "flavor"))
	if flavor == "" {
		flavor = "dds.mongodb.s6.large.2.shard"
	}
	// Default mode is "Sharding" when absent from plan.
	_ = getStr(r.After, "mode")

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "dds",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": []map[string]interface{}{
				huaweiProductInfo("hws.service.type.dds", "hws.resource.type.dds", flavor, r.Region, 0, 0),
			},
		},
	}, nil
}

func (HuaweiDDS) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei DDS",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
