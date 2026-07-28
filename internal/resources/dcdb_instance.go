package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// DCDBInstance handles `tencentcloud_dcdb_db_instance` (TDSQL MySQL, PREPAID)
// and `tencentcloud_dcdb_hourdb_instance` (POSTPAID).
//
// Pricing API (dcdb): DescribeDCDBPrice.
// Docs: https://cloud.tencent.com/document/product/557/16131
//
// The TF provider does not have a "charge_type" field — the billing mode is
// determined by the resource type itself:
//   - tencentcloud_dcdb_db_instance   → prepaid  (has "period" field)
//   - tencentcloud_dcdb_hourdb_instance → postpaid (no period field)
//
// We request AmountUnit="pent" so Response.{OriginalPrice,Price} come back as
// int64 cents (value/100 = CNY). For PREPAID the value is the total for the
// requested Period (cloudtab forces Period=1 → monthly); for POSTPAID it is
// an hourly rate.
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
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_dcdb requires zones/shard_memory/shard_storage/shard_count")
	}

	shardNodeCount := getInt(r.After, "shard_node_count")
	if shardNodeCount <= 0 {
		shardNodeCount = 2 // one master + one replica is the DCDB default
	}
	count := getInt(r.After, "count")
	if count <= 0 {
		count = 1
	}

	// Billing mode is determined by the TF resource type:
	//   tencentcloud_dcdb_db_instance    → prepaid
	//   tencentcloud_dcdb_hourdb_instance → postpaid
	payMode := "postpaid"
	if strings.Contains(r.Type, "hourdb") {
		payMode = "postpaid"
	} else {
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
	}
	// Period is required by the API for both modes; cloudtab always prices a
	// single month (Period=1 → monthly run-rate).
	params["Period"] = 1

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
		Currency:    tencentCurrency(req.ExpectedCurrency),
	}}, nil
}
