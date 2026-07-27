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
				alibabaModule("InstanceClass", "Hour", "InstanceClass:"+class),
			},
		},
	}, nil
}

func (AlibabaRedis) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw, req.ExpectedCurrency)
	if err != nil {
		return nil, err
	}
	return simpleHourlyCost("Alibaba Redis", info.PriceYuan, info.Currency), nil
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
				alibabaModule("DBInstanceClass", "Hour", "DBInstanceClass:"+class),
				alibabaModule("DBInstanceStorage", "Hour", fmt.Sprintf("DBInstanceStorage:%d", storage)),
			},
		},
	}, nil
}

func (AlibabaMongoDB) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw, req.ExpectedCurrency)
	if err != nil {
		return nil, err
	}
	return simpleHourlyCost("Alibaba MongoDB", info.PriceYuan, info.Currency), nil
}
