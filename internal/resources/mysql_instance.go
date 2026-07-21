package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// MySQLInstance handles `tencentcloud_mysql_instance`.
//
// Pricing API (cdb): DescribeDBPrice.
// Docs: https://cloud.tencent.com/document/api/236/15866
//
// Terraform provider fields commonly seen:
// - availability_zone, mem_size, volume_size, charge_type, prepaid_period
// - cpu, instance_role, device_type
//
// We normalize charge_type to cdb.PayType:
// - PREPAID  -> PRE_PAID
// - POSTPAID -> HOUR_PAID
// and treat pay-as-you-go as hourly for monthly estimation (x730).
type MySQLInstance struct{}

func (MySQLInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	getStr := func(k string) string {
		if v, ok := r.After[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	getInt := func(k string) int64 {
		switch v := r.After[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		}
		return 0
	}

	zone := getStr("availability_zone")
	if zone == "" {
		zone = getStr("zone")
	}
	memory := getInt("mem_size")
	if memory == 0 {
		memory = getInt("memory")
	}
	volume := getInt("volume_size")
	if volume == 0 {
		volume = getInt("volume")
	}
	if zone == "" || memory == 0 || volume == 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_mysql_instance requires availability_zone/mem_size/volume_size")
	}

	goodsNum := getInt("count")
	if goodsNum <= 0 {
		goodsNum = 1
	}
	instanceRole := strings.ToLower(getStr("instance_role"))
	if instanceRole == "" {
		instanceRole = "master"
	}

	chargeType := strings.ToUpper(getStr("charge_type"))
	payType := "HOUR_PAID"
	period := int64(1)
	if p := getInt("prepaid_period"); p > 0 {
		period = p
	} else if p := getInt("period"); p > 0 {
		period = p
	}
	if chargeType == "PREPAID" || chargeType == "PRE_PAID" {
		payType = "PRE_PAID"
		if period <= 0 {
			period = 1
		}
	}

	params := map[string]interface{}{
		"Zone":         zone,
		"Memory":       memory,
		"Volume":       volume,
		"GoodsNum":     goodsNum,
		"InstanceRole": instanceRole,
		"PayType":      payType,
		"Period":       period,
	}
	if cpu := getInt("cpu"); cpu > 0 {
		params["Cpu"] = cpu
	}
	if deviceType := getStr("device_type"); deviceType != "" {
		params["DeviceType"] = strings.ToUpper(deviceType)
	}
	if nodes := getInt("instance_nodes"); nodes > 0 {
		params["InstanceNodes"] = nodes
	}

	return pricing.PriceRequest{
		Product: "cdb",
		Action:  "DescribeDBPrice",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (MySQLInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		Price         float64 `json:"Price"`
		OriginalPrice float64 `json:"OriginalPrice"`
		Currency      string  `json:"Currency"`
		Response      struct {
			Price         float64 `json:"Price"`
			OriginalPrice float64 `json:"OriginalPrice"`
			Currency      string  `json:"Currency"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	priceFen := wrap.Price
	currency := wrap.Currency
	if wrap.Response.Price > 0 {
		priceFen = wrap.Response.Price
	}
	if wrap.Response.Currency != "" {
		currency = wrap.Response.Currency
	}
	if currency == "" {
		currency = "CNY"
	}

	priceYuan := priceFen / 100.0
	payType := fmt.Sprintf("%v", req.Params["PayType"])
	monthly := priceYuan
	hourly := 0.0
	if payType == "HOUR_PAID" {
		hourly = priceYuan
		monthly = priceYuan * hoursPerMonth
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("MySQL (%vMB/%vGB)", req.Params["Memory"], req.Params["Volume"]),
		Unit:        payType,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    currency,
	}}, nil
}
