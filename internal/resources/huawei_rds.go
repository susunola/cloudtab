package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiRDS handles `huaweicloud_rds_instance` (RDS).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiRDS struct{}

func (HuaweiRDS) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	// The RDS flavor (e.g. "rds.mysql.c2.large") already encodes the engine, so
	// the DB engine type is not a separate pricing input here.
	flavor := strings.TrimSpace(getStr(r.After, "flavor"))
	if flavor == "" {
		flavor = "rds.mysql.c2.large"
	}

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "rds",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": []map[string]interface{}{
				huaweiProductInfo("hws.service.type.rds", "hws.resource.type.database", flavor, r.Region, 0, 0),
			},
		},
	}, nil
}

func (HuaweiRDS) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei RDS",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
