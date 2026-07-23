package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiELB handles `huaweicloud_elb_loadbalancer` (ELB).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiELB struct{}

func (HuaweiELB) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	lbType := strings.TrimSpace(getStr(r.After, "type"))
	if lbType == "" {
		lbType = "L7"
	}

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "elb",
		Region:   r.Region,
		Params: map[string]interface{}{
			"project_id": r.Region,
			"product_infos": []map[string]interface{}{
				{
					"id":                 "1",
					"cloud_service_type": "hws.service.type.elb",
					"resource_type":      "hws.resource.type.elb",
					"resource_spec":      lbType,
					"region":             r.Region,
					"usage_factor":       "1",
					"usage_value":        1,
					"usage_measure_id":   1,
				},
			},
		},
	}, nil
}

func (HuaweiELB) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        fmt.Sprintf("Huawei ELB"),
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
