package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaDisk handles `alicloud_disk` (cloud disk / EBS equivalent).
type AlibabaDisk struct{}

func (AlibabaDisk) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	category := strings.TrimSpace(getStr(r.After, "category"))
	if category == "" {
		category = "cloud_essd"
	}
	size := getInt(r.After, "size")
	if size <= 0 {
		size = 40
	}
	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "disk",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList": []map[string]string{
				alibabaModule("DiskSize", "Hour", fmt.Sprintf("%s:%d", category, size)),
			},
		},
	}, nil
}

func (AlibabaDisk) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba Disk",
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
