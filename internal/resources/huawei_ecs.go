package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiECS handles `huaweicloud_compute_instance` (ECS).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiECS struct{}

func (HuaweiECS) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	flavorID := strings.TrimSpace(getStr(r.After, "flavor_id"))
	if flavorID == "" {
		return pricing.PriceRequest{}, fmt.Errorf("huaweicloud_compute_instance requires flavor_id")
	}

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "ecs",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": []map[string]interface{}{
				huaweiProductInfo("hws.service.type.ec2", "hws.resource.type.vm", flavorID, r.Region, 0, 0),
			},
		},
	}, nil
}

func (HuaweiECS) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei ECS",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
