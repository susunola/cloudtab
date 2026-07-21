package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// SQLServerInstance handles `tencentcloud_sqlserver_instance`.
//
// Pricing API (sqlserver): InquiryPriceCreateDBInstances.
// Docs: https://cloud.tencent.com/document/api/238/19992
//
// Response.{OriginalPrice,Price} are int64 in 分 (value/100 = 元). For PREPAID
// the value is the total for the requested Period; for POSTPAID it is an hourly
// rate. cloudtab always prices a single month (Period=1) so PREPAID == monthly.
type SQLServerInstance struct{}

func (SQLServerInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zone := strings.TrimSpace(getStr(r.After, "availability_zone"))
	if zone == "" {
		zone = getStr(r.After, "zone")
	}
	memory := getInt(r.After, "memory")
	storage := getInt(r.After, "storage")
	if zone == "" || memory <= 0 || storage <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_sqlserver_instance requires availability_zone/memory/storage")
	}

	chargeType := strings.ToUpper(strings.TrimSpace(getStr(r.After, "charge_type")))
	if chargeType == "" {
		chargeType = strings.ToUpper(strings.TrimSpace(getStr(r.After, "instance_charge_type")))
	}
	switch chargeType {
	case "PREPAID", "POSTPAID":
		// keep as-is
	case "POSTPAID_BY_HOUR":
		chargeType = "POSTPAID"
	default:
		chargeType = "POSTPAID"
	}

	goodsNum := getInt(r.After, "count")
	if goodsNum <= 0 {
		goodsNum = 1
	}

	params := map[string]interface{}{
		"Zone":               zone,
		"Memory":             memory,
		"Storage":            storage,
		"InstanceChargeType": chargeType,
		"GoodsNum":           goodsNum,
		// Always price a single month; cloudtab reports a monthly run-rate.
		"Period": 1,
	}
	if cpu := getInt(r.After, "cpu"); cpu > 0 {
		params["Cpu"] = cpu
	}
	if v := getStr(r.After, "engine_version"); v != "" {
		params["DBVersion"] = v
	}
	if t := getStr(r.After, "instance_type"); t != "" {
		params["InstanceType"] = t
	}
	if m := getStr(r.After, "machine_type"); m != "" {
		params["MachineType"] = m
	}

	return pricing.PriceRequest{
		Product: "sqlserver",
		Action:  "InquiryPriceCreateDBInstances",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (SQLServerInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
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

	priceFen := wrap.Price
	origFen := wrap.OriginalPrice
	if wrap.Response.Price > 0 || wrap.Response.OriginalPrice > 0 {
		priceFen = wrap.Response.Price
		origFen = wrap.Response.OriginalPrice
	}
	// Prefer the discounted price; fall back to the original.
	if priceFen == 0 {
		priceFen = origFen
	}
	priceYuan := float64(priceFen) / 100.0

	chargeType := strings.ToUpper(fmt.Sprintf("%v", req.Params["InstanceChargeType"]))
	monthly := priceYuan
	hourly := 0.0
	if chargeType != "PREPAID" { // POSTPAID: value is an hourly rate
		hourly = priceYuan
		monthly = hourly * hoursPerMonth
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("SQL Server (%vGB mem, %vGB disk)", req.Params["Memory"], req.Params["Storage"]),
		Unit:        chargeType,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
