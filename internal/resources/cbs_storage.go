package resources

import (
	"encoding/json"
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// CBSStorage handles `tencentcloud_cbs_storage`.
//
// Reference: https://cloud.tencent.com/document/api/362/16315
// (InquiryPriceCreateDisks → Response.DiskPrice.OriginalPrice / DiscountPrice)
type CBSStorage struct{}

func (CBSStorage) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	getStr := func(k string) string {
		if v, ok := r.After[k].(string); ok {
			return v
		}
		return ""
	}
	getInt := func(k string) int64 {
		switch v := r.After[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		}
		return 0
	}

	diskType := getStr("storage_type")
	if diskType == "" {
		diskType = getStr("disk_type")
	}
	size := getInt("storage_size")
	if size == 0 {
		size = getInt("disk_size")
	}
	zone := getStr("availability_zone")
	if diskType == "" || size == 0 || zone == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_cbs_storage requires storage_type/storage_size/availability_zone")
	}

	chargeType := getStr("charge_type")
	if chargeType == "" {
		chargeType = "POSTPAID_BY_HOUR"
	}

	params := map[string]interface{}{
		"DiskType":         diskType,
		"DiskSize":         size,
		"DiskChargeType":   chargeType,
		"DiskCount":        1,
		"Placement":        map[string]interface{}{"Zone": zone},
	}
	if chargeType == "PREPAID" {
		params["DiskChargePrepaid"] = map[string]interface{}{
			"Period": getInt("prepaid_period"),
		}
	}

	return pricing.PriceRequest{
		Product: "cbs",
		Action:  "InquiryPriceCreateDisks",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (CBSStorage) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		DiskPrice struct {
			UnitPrice         float64 `json:"UnitPrice"`
			UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
			OriginalPrice     float64 `json:"OriginalPrice"`
			DiscountPrice     float64 `json:"DiscountPrice"`
			ChargeUnit        string  `json:"ChargeUnit"`
		} `json:"DiskPrice"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	dp := wrap.DiskPrice
	monthly, hourly := monthlyFromPrice(dp.ChargeUnit, dp.UnitPriceDiscount, dp.DiscountPrice)
	return []output.CostComponent{{
		Name:        fmt.Sprintf("CBS %v (%vGB)", req.Params["DiskType"], req.Params["DiskSize"]),
		Unit:        dp.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
