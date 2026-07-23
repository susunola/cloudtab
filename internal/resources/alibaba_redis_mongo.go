package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaRedis handles `alicloud_kvstore_instance` (ApsaraDB for Redis / Tair).
type AlibabaRedis struct{}

func (AlibabaRedis) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	class := strings.TrimSpace(getStr(r.After, "instance_class"))
	if class == "" {
		class = "redis.master.small.default"
	}
	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "redisa", // BSS product code for Redis
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList": []map[string]string{
				alibabaModule("InstanceClass", "Hour", class),
			},
		},
	}, nil
}

func (AlibabaRedis) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba Redis",
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}

// AlibabaMongoDB handles `alicloud_mongodb_instance` (ApsaraDB for MongoDB).
type AlibabaMongoDB struct{}

func (AlibabaMongoDB) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	class := strings.TrimSpace(getStr(r.After, "db_instance_class"))
	if class == "" {
		class = "dds.mongo.mid"
	}
	storage := getInt(r.After, "db_instance_storage")
	if storage <= 0 {
		storage = 20
	}
	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "dds", // BSS product code for MongoDB
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList": []map[string]string{
				alibabaModule("DBInstanceClass", "Hour", fmt.Sprintf("%s:%d", class, storage)),
			},
		},
	}, nil
}

func (AlibabaMongoDB) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba MongoDB",
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
