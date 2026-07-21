package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// ECMInstance handles `tencentcloud_ecm_instance` (Edge Computing Machine).
//
// Pricing API (ecm): DescribePriceRunInstance.
// Docs: https://cloud.tencent.com/document/api/1108/42030
//
// InstanceChargeType is an int enum: 0 = resource-postpaid, 1 = hourly-postpaid,
// 2 = monthly-postpaid. ECM has no PREPAID mode. Response.InstancePrice.
// {OriginalPrice,DiscountPrice} are uint64 in 分 (value/100 = 元); the value is
// an hourly rate for the hourly charge types.
type ECMInstance struct{}

func (ECMInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := strings.TrimSpace(getStr(r.After, "instance_type"))
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_ecm_instance requires instance_type")
	}

	count := getInt(r.After, "instance_count")
	if count <= 0 {
		count = 1
	}

	// ECM only has postpaid modes; default to hourly-postpaid (1).
	chargeType := 1

	params := map[string]interface{}{
		"InstanceType":       instanceType,
		"InstanceCount":      count,
		"InstanceChargeType": chargeType,
	}

	// System disk is a nested object; only send it when a size is present so we
	// never build a half-populated disk spec.
	if size := getInt(r.After, "system_disk_size"); size > 0 {
		disk := map[string]interface{}{"DiskSize": size}
		if dt := strings.TrimSpace(getStr(r.After, "system_disk_type")); dt != "" {
			disk["DiskType"] = dt
		}
		params["SystemDisk"] = disk
	}

	return pricing.PriceRequest{
		Product: "ecm",
		Action:  "DescribePriceRunInstance",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (ECMInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	type instancePrice struct {
		OriginalPrice uint64 `json:"OriginalPrice"`
		DiscountPrice uint64 `json:"DiscountPrice"`
	}
	var wrap struct {
		InstancePrice instancePrice `json:"InstancePrice"`
		Response      struct {
			InstancePrice instancePrice `json:"InstancePrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	ip := wrap.InstancePrice
	if wrap.Response.InstancePrice.OriginalPrice > 0 || wrap.Response.InstancePrice.DiscountPrice > 0 {
		ip = wrap.Response.InstancePrice
	}

	// Prefer the discounted price; fall back to the original. Values are 分.
	hourly := preferDiscount(float64(ip.DiscountPrice), float64(ip.OriginalPrice)) / 100.0

	return []output.CostComponent{{
		Name:        fmt.Sprintf("ECM (%v)", req.Params["InstanceType"]),
		Unit:        "HOUR",
		HourlyCost:  hourly,
		MonthlyCost: hourly * hoursPerMonth,
		Currency:    "CNY",
	}}, nil
}
