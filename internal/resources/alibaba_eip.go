package resources

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaEIP handles `alicloud_eip` (elastic IP).
type AlibabaEIP struct{}

func (AlibabaEIP) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	bandwidth := getInt(r.After, "bandwidth")
	if bandwidth <= 0 {
		bandwidth = 1
	}
	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "eip",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"Quantity":         1,
			"ModuleList": []map[string]string{
				{"ModuleCode": "Bandwidth", "PriceType": "Hour", "Config": fmt.Sprintf("%d:Mbps", bandwidth)},
			},
		},
	}, nil
}

func (AlibabaEIP) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba EIP",
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
