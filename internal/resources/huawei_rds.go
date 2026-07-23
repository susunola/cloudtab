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
	flavor := strings.TrimSpace(getStr(r.After, "flavor"))
	if flavor == "" {
		flavor = "rds.mysql.c2.large"
	}
	// Default db type is "MySQL" when absent from plan.
	_ = getStr(r.After, "db.0.type")

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "rds",
		Region:   r.Region,
		Params: map[string]interface{}{
			"project_id": r.Region,
			"product_infos": []map[string]interface{}{
				{
					"id":                 "1",
					"cloud_service_type": "hws.service.type.rds",
					"resource_type":      "hws.resource.type.database",
					"resource_spec":      flavor,
					"region":             r.Region,
					"usage_factor":       "1",
					"usage_value":        1,
					"usage_measure_id":   1,
				},
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
