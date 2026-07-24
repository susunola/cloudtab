package resources

import (
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
			"product_infos": []map[string]interface{}{
				huaweiProductInfo("hws.service.type.elb", "hws.resource.type.elb", lbType, r.Region, 0, 0),
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
		Name:        "Huawei ELB",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
