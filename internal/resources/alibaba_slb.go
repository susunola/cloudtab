package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaSLB handles `alicloud_slb_load_balancer` (Server Load Balancer, CLB/ALB).
type AlibabaSLB struct{}

func (AlibabaSLB) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	spec := strings.TrimSpace(getStr(r.After, "specification"))
	if spec == "" {
		spec = "slb.s1.small"
	}
	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "slb",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"Quantity":         1,
			"ModuleList": []map[string]string{
				{"ModuleCode": "Specification", "PriceType": "Hour", "Config": spec},
			},
		},
	}, nil
}

func (AlibabaSLB) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        fmt.Sprintf("Alibaba SLB"),
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
