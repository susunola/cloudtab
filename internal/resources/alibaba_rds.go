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
			"ModuleList": []map[string]string{
				alibabaModule("DBInstanceClass", "Hour", "DBInstanceClass:"+class),
				alibabaModule("DBInstanceStorage", "Hour", fmt.Sprintf("DBInstanceStorage:%d", storage)),
			},
		},
	}, nil
}

func (AlibabaRDS) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw, req.ExpectedCurrency)
	if err != nil {
		return nil, err
	}
	return simpleHourlyCost("Alibaba RDS", info.PriceYuan, info.Currency), nil
}
