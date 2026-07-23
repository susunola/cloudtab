package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiCCE handles `huaweicloud_cce_cluster` (CCE).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiCCE struct{}

func (HuaweiCCE) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	clusterType := strings.TrimSpace(getStr(r.After, "cluster_type"))
	if clusterType == "" {
		clusterType = "VirtualMachine"
	}
	// Default cluster_version is "v1.27" when absent from plan.
	_ = getStr(r.After, "cluster_version")

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "cce",
		Region:   r.Region,
		Params: map[string]interface{}{
			"project_id": r.Region,
			"product_infos": []map[string]interface{}{
				{
					"id":                 "1",
					"cloud_service_type": "hws.service.type.cce",
					"resource_type":      "hws.resource.type.cce",
					"resource_spec":      clusterType,
					"region":             r.Region,
					"usage_factor":       "1",
					"usage_value":        1,
					"usage_measure_id":   1,
				},
			},
		},
	}, nil
}

func (HuaweiCCE) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei CCE",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
