package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// MariaDBInstance handles `tencentcloud_mariadb_instance`.
//
// Pricing API (mariadb): DescribePrice.
// Docs: https://cloud.tencent.com/document/api/237/16177
//
// Terraform provider fields commonly seen:
//   - zones (first used as Zone), node_count, memory (GB), storage (GB),
//     period (months), instance_charge_type / charge_type
//
// Response.{Price,OriginalPrice} are int64 in 分 (cents). We request
// AmountUnit="pent" for an explicit unit and divide by 100 to get 元. The
// returned figure is the total for the requested Period, so PREPAID is a
// period total and POSTPAID is an hourly rate.
type MariaDBInstance struct{}

func (MariaDBInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zone := strings.TrimSpace(getStr(r.After, "availability_zone"))
	if zone == "" {
		zone = firstZone(r.After)
	}
	memory := getInt(r.After, "memory")
	storage := getInt(r.After, "storage")
	if zone == "" || memory <= 0 || storage <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_mariadb_instance requires zone/memory/storage")
	}

	nodeCount := getInt(r.After, "node_count")
	if nodeCount <= 0 {
		nodeCount = 2 // one primary + one replica default
	}
	count := getInt(r.After, "count")
	if count <= 0 {
		count = 1
	}

	payMode := strings.ToLower(strings.TrimSpace(getStr(r.After, "instance_charge_type")))
	if payMode == "" {
		payMode = strings.ToLower(strings.TrimSpace(getStr(r.After, "charge_type")))
	}
	// mariadb Paymode expects "prepaid" | "postpaid".
	switch {
	case strings.Contains(payMode, "postpaid"), payMode == "":
		payMode = "postpaid"
	default:
		payMode = "prepaid"
	}

	// Always price a single month (Period=1): cloudtab reports a monthly
	// run-rate, so a PREPAID period total for the user's real term would need
	// to be divided back down. Asking for one month keeps the returned price
	// equal to the monthly cost by construction.
	params := map[string]interface{}{
		"Zone":       zone,
		"NodeCount":  nodeCount,
		"Memory":     memory,
		"Storage":    storage,
		"Period":     1,
		"Count":      count,
		"Paymode":    payMode,
		"AmountUnit": "pent", // return price in 分 for a stable integer unit
	}

	return pricing.PriceRequest{
		Product: "mariadb",
		Action:  "DescribePrice",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (MariaDBInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		Price         int64 `json:"Price"`
		OriginalPrice int64 `json:"OriginalPrice"`
		Response      struct {
			Price         int64 `json:"Price"`
			OriginalPrice int64 `json:"OriginalPrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	priceFen := wrap.Price
	originalFen := wrap.OriginalPrice
	if wrap.Response.Price > 0 || wrap.Response.OriginalPrice > 0 {
		priceFen = wrap.Response.Price
		originalFen = wrap.Response.OriginalPrice
	}
	// Prefer the discounted price; fall back to the undiscounted original when
	// the API returns no discount (mirrors the postgres/mysql mappers).
	if priceFen == 0 {
		priceFen = originalFen
	}
	priceYuan := float64(priceFen) / 100.0

	payMode := strings.ToLower(fmt.Sprintf("%v", req.Params["Paymode"]))
	monthly := priceYuan
	hourly := 0.0
	if strings.Contains(payMode, "postpaid") {
		// POSTPAID DescribePrice returns an hourly rate.
		hourly = priceYuan
		monthly = priceYuan * hoursPerMonth
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("MariaDB (%vGB mem, %vGB disk)", req.Params["Memory"], req.Params["Storage"]),
		Unit:        strings.ToUpper(payMode),
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
