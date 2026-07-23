package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaRDS handles `alicloud_db_instance` (RDS).
type AlibabaRDS struct{}

func (AlibabaRDS) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	engine := strings.TrimSpace(getStr(r.After, "engine"))
	if engine == "" {
		engine = "MySQL"
	}
	class := strings.TrimSpace(getStr(r.After, "instance_type"))
	if class == "" {
		return pricing.PriceRequest{}, fmt.Errorf("alicloud_db_instance requires instance_type")
	}
	storage := getInt(r.After, "instance_storage")
	if storage <= 0 {
		storage = 40
	}

	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "rds",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"Quantity":         1,
			"ModuleList": []map[string]string{
				{"ModuleCode": "DBInstanceClass", "PriceType": "Hour", "Config": fmt.Sprintf("%s:%s:%d", strings.ToLower(engine), class, storage)},
			},
		},
	}, nil
}

func (AlibabaRDS) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba RDS",
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
