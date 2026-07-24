package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// DCDBInstance handles `tencentcloud_dcdb_instance` (TDSQL MySQL, distributed).
//
// Pricing API (dcdb): DescribeDCDBPrice.
// Docs: https://cloud.tencent.com/document/api/557/16135
//
// We request AmountUnit="pent" so Response.{OriginalPrice,Price} come back as
// int64 cents (value/100 = CNY). Paymode is the lowercase "prepaid"/"postpaid".
// For PREPAID the value is the total for the requested Period; for POSTPAID it
// is an hourly rate. cloudtab always prices a single month (Period=1).
type DCDBInstance struct{}

func (DCDBInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zone := strings.TrimSpace(getStr(r.After, "availability_zone"))
	if zone == "" {
		zone = firstZone(r.After)
	}
	if zone == "" {
		zone = getStr(r.After, "zone")
	}
	shardMemory := getInt(r.After, "shard_memory")
	shardStorage := getInt(r.After, "shard_storage")
	shardCount := getInt(r.After, "shard_count")
	if zone == "" || shardMemory <= 0 || shardStorage <= 0 || shardCount <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_dcdb_instance requires availability_zone/shard_memory/shard_storage/shard_count")
	}

	shardNodeCount := getInt(r.After, "shard_node_count")
	if shardNodeCount <= 0 {
		shardNodeCount = 2 // one master + one replica is the DCDB default
	}
	count := getInt(r.After, "count")
	if count <= 0 {
		count = 1
	}

	// Terraform stores charge type as PREPAID / POSTPAID (or *_BY_HOUR); the DCDB
	// API expects the lowercase "prepaid" / "postpaid".
	payMode := "postpaid"
	ct := strings.ToUpper(strings.TrimSpace(getStr(r.After, "instance_charge_type")))
	if ct == "" {
		ct = strings.ToUpper(strings.TrimSpace(getStr(r.After, "charge_type")))
	}
	if strings.HasPrefix(ct, "PREPAID") {
		payMode = "prepaid"
	}

	params := map[string]interface{}{
		"Zone":           zone,
		"Count":          count,
		"ShardNodeCount": shardNodeCount,
		"ShardMemory":    shardMemory,
		"ShardStorage":   shardStorage,
		"ShardCount":     shardCount,
		"Paymode":        payMode,
		"AmountUnit":     "pent", // return price in cents for a stable integer unit
		// Always price a single month; cloudtab reports a monthly run-rate.
		"Period": 1,
	}

	return pricing.PriceRequest{
		Product: "dcdb",
		Action:  "DescribeDCDBPrice",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (DCDBInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		OriginalPrice int64 `json:"OriginalPrice"`
		Price         int64 `json:"Price"`
		Response      struct {
			OriginalPrice int64 `json:"OriginalPrice"`
			Price         int64 `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	priceYuan := discountedYuanFromCents(
		wrap.Price, wrap.OriginalPrice,
		wrap.Response.Price, wrap.Response.OriginalPrice,
	)

	payMode := strings.ToLower(fmt.Sprintf("%v", req.Params["Paymode"]))
	// postpaid: value is an hourly rate; prepaid: value is the monthly total.
	monthly, hourly := splitByBilling(priceYuan, payMode != "prepaid")

	return []output.CostComponent{{
		Name:        fmt.Sprintf("TDSQL MySQL (%v shards, %vGB mem)", req.Params["ShardCount"], req.Params["ShardMemory"]),
		Unit:        strings.ToUpper(payMode),
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
