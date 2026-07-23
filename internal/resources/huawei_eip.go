package resources

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiEIP handles `huaweicloud_vpc_eip` (elastic IP / bandwidth).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiEIP struct{}

func (HuaweiEIP) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "eip",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": []map[string]interface{}{
				huaweiProductInfo("hws.service.type.vpc", "hws.resource.type.bandwidth", "bandwidth", r.Region, 0, 0),
			},
		},
	}, nil
}

func (HuaweiEIP) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        fmt.Sprintf("Huawei EIP"),
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
