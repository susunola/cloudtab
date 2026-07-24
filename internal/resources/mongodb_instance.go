package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// MongoDBInstance handles `tencentcloud_mongodb_instance`.
//
// Pricing API (mongodb): InquirePriceCreateDBInstances (SDK spells it
// "InquirePrice", without the 'y').
// Docs: https://cloud.tencent.com/document/api/240/38571
//
// Terraform provider fields commonly seen:
//   - available_zone, memory (GB), volume (GB), engine_version,
//     machine_type, charge_type, prepaid_period, node_num
//
// Response.Price.{UnitPrice,OriginalPrice,DiscountPrice} is in CNY. There is no
// ChargeUnit field: PREPAID prices are a monthly total, POSTPAID an hourly rate,
// decided from the charge type in the request.
type MongoDBInstance struct{}

func (MongoDBInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zone := strings.TrimSpace(getStr(r.After, "available_zone"))
	if zone == "" {
		zone = getStr(r.After, "availability_zone")
	}
	memory := getInt(r.After, "memory")
	volume := getInt(r.After, "volume")
	if zone == "" || memory <= 0 || volume <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_mongodb_instance requires available_zone/memory/volume")
	}

	nodeNum := getInt(r.After, "node_num")
	if nodeNum <= 0 {
		nodeNum = 3 // replica-set default
	}
	goodsNum := getInt(r.After, "count")
	if goodsNum <= 0 {
		goodsNum = 1
	}

	chargeType := strings.ToUpper(strings.TrimSpace(getStr(r.After, "charge_type")))
	if chargeType == "" {
		chargeType = strings.ToUpper(strings.TrimSpace(getStr(r.After, "instance_charge_type")))
	}
	if chargeType == "" {
		chargeType = "POSTPAID_BY_HOUR"
	}

	// cloudtab reports a monthly run-rate, so PREPAID is always priced for a
	// single month (Period=1). Pricing the user's real multi-month term would
	// return a period total that we would then have to divide back down; asking
	// for one month keeps "returned price == monthly cost" true by construction.
	params := map[string]interface{}{
		"Zone":               zone,
		"NodeNum":            nodeNum,
		"Memory":             memory,
		"Volume":             volume,
		"GoodsNum":           goodsNum,
		"Period":             1,
		"InstanceChargeType": chargeType,
	}
	if v := getStr(r.After, "engine_version"); v != "" {
		params["MongoVersion"] = v
	}
	if m := getStr(r.After, "machine_type"); m != "" {
		params["MachineCode"] = m
	}
	// The Terraform provider uses separate resource types for replica-set vs
	// sharding clusters and does NOT expose a "cluster_type" attribute.
	//   - tencentcloud_mongodb_instance            → REPLSET
	//   - tencentcloud_mongodb_sharding_instance    → SHARD
	// We read the field first for forward-compat, then fall back to REPLSET
	// since this handler is registered for tencentcloud_mongodb_instance.
	if c := getStr(r.After, "cluster_type"); c != "" {
		params["ClusterType"] = c
	} else {
		params["ClusterType"] = "REPLSET"
	}

	// ReplicateSetNum is required by InquirePriceCreateDBInstances.
	// For REPLSET (tencentcloud_mongodb_instance) it is always 1.
	params["ReplicateSetNum"] = 1

	return pricing.PriceRequest{
		Product: "mongodb",
		Action:  "InquirePriceCreateDBInstances",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (MongoDBInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	type dbPrice struct {
		UnitPrice     float64 `json:"UnitPrice"`
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
	}
	var wrap struct {
		Price    dbPrice `json:"Price"`
		Response struct {
			Price dbPrice `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	p := wrap.Price
	if wrap.Response.Price.DiscountPrice > 0 || wrap.Response.Price.UnitPrice > 0 {
		p = wrap.Response.Price
	}

	// Prefer the discounted price; fall back to the original if no discount given.
	priceYuan := p.DiscountPrice
	if priceYuan == 0 {
		priceYuan = p.OriginalPrice
	}

	chargeType := strings.ToUpper(fmt.Sprintf("%v", req.Params["InstanceChargeType"]))
	monthly := priceYuan
	hourly := 0.0
	// UnitPrice is the hourly rate for POSTPAID; use it when billing hourly.
	if strings.Contains(chargeType, "HOUR") || chargeType == "POSTPAID" {
		hourly = p.UnitPrice
		if hourly == 0 {
			hourly = priceYuan
		}
		monthly = hourly * hoursPerMonth
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("MongoDB (%vGB mem, %vGB disk)", req.Params["Memory"], req.Params["Volume"]),
		Unit:        chargeType,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
