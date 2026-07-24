package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// RedisInstance handles `tencentcloud_redis_instance`.
//
// Pricing API (redis): InquiryPriceCreateInstance.
// Docs: https://cloud.tencent.com/document/api/239/41159
//
// Terraform provider fields commonly seen:
// - availability_zone, type_id, mem_size, charge_type, prepaid_period
// - redis_shard_num, redis_replicas_num, product_version
//
// BillingMode mapping:
// - POSTPAID -> 0 (pay-as-you-go, treated as hourly for monthly estimate)
// - PREPAID  -> 1 (fixed period price)
type RedisInstance struct{}

func (RedisInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zoneName := strings.TrimSpace(getStr(r.After, "availability_zone"))
	if zoneName == "" {
		zoneName = getStr(r.After, "zone")
	}
	memSize := getInt(r.After, "mem_size")
	typeID := getInt(r.After, "type_id")
	if typeID == 0 {
		typeID = 6 // default to Redis 4.0 standard for missing plan details
	}
	if zoneName == "" || memSize == 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_redis_instance requires availability_zone/type_id/mem_size")
	}

	goodsNum := getInt(r.After, "count")
	if goodsNum <= 0 {
		goodsNum = 1
	}
	chargeType := strings.ToUpper(getStr(r.After, "charge_type"))
	billingMode := int64(0)
	if chargeType == "PREPAID" || chargeType == "PRE_PAID" {
		billingMode = 1
	}

	params := map[string]interface{}{
		"TypeId":   typeID,
		"MemSize":  memSize,
		"GoodsNum": goodsNum,
		// Always price a single month: cloudtab reports a monthly run-rate and
		// the PREPAID (BillingMode=1) price is a period total, so Period=1
		// keeps it monthly.
		"Period":      1,
		"BillingMode": billingMode,
		"ZoneName":    zoneName,
	}
	if shards := getInt(r.After, "redis_shard_num"); shards > 0 {
		params["RedisShardNum"] = shards
	}
	if replicas := getInt(r.After, "redis_replicas_num"); replicas > 0 {
		params["RedisReplicasNum"] = replicas
	}
	if pv := getStr(r.After, "product_version"); pv != "" {
		params["ProductVersion"] = pv
	}
	if v := getBool(r.After, "replicas_readonly"); v {
		params["ReplicasReadonly"] = v
	}

	return pricing.PriceRequest{
		Product: "redis",
		Action:  "InquiryPriceCreateInstance",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (RedisInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		Price              float64 `json:"Price"`
		HighPrecisionPrice float64 `json:"HighPrecisionPrice"`
		Currency           string  `json:"Currency"`
		AmountUnit         string  `json:"AmountUnit"`
		Response           struct {
			Price              float64 `json:"Price"`
			HighPrecisionPrice float64 `json:"HighPrecisionPrice"`
			Currency           string  `json:"Currency"`
			AmountUnit         string  `json:"AmountUnit"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	price := wrap.Price
	if wrap.HighPrecisionPrice > 0 {
		price = wrap.HighPrecisionPrice
	}
	currency := wrap.Currency
	amountUnit := wrap.AmountUnit
	if wrap.Response.Price > 0 {
		price = wrap.Response.Price
	}
	if wrap.Response.HighPrecisionPrice > 0 {
		price = wrap.Response.HighPrecisionPrice
	}
	if wrap.Response.Currency != "" {
		currency = wrap.Response.Currency
	}
	if wrap.Response.AmountUnit != "" {
		amountUnit = wrap.Response.AmountUnit
	}
	if currency == "" {
		currency = "CNY"
	}

	priceYuan := normalizeTencentAmount(price, amountUnit)
	billingMode := fmt.Sprintf("%v", req.Params["BillingMode"])
	monthly := priceYuan
	hourly := 0.0
	if billingMode == "0" {
		hourly = priceYuan
		monthly = priceYuan * hoursPerMonth
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("Redis (type %v, %vMB)", req.Params["TypeId"], req.Params["MemSize"]),
		Unit:        map[string]string{"0": "HOUR", "1": "MONTH"}[billingMode],
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    currency,
	}}, nil
}

func normalizeTencentAmount(price float64, amountUnit string) float64 {
	switch strings.ToLower(strings.TrimSpace(amountUnit)) {
	case "pent":
		return price / 100.0
	case "micropent":
		return price / 100000000.0
	default:
		// Some APIs return CNY directly (or omit AmountUnit). Keep original value.
		return price
	}
}
