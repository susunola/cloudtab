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
	diskType := getStr(r.After, "storage_type")
	if diskType == "" {
		diskType = getStr(r.After, "disk_type")
	}
	size := getInt(r.After, "storage_size")
	if size == 0 {
		size = getInt(r.After, "disk_size")
	}
	zone := getStr(r.After, "availability_zone")
	if diskType == "" || size == 0 || zone == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_cbs_storage requires storage_type/storage_size/availability_zone")
	}

	chargeType := getStr(r.After, "charge_type")
	if chargeType == "" {
		chargeType = "POSTPAID_BY_HOUR"
	}

	params := map[string]interface{}{
		"DiskType":       diskType,
		"DiskSize":       size,
		"DiskChargeType": chargeType,
		"DiskCount":      1,
		"Placement":      map[string]interface{}{"Zone": zone},
	}
	if chargeType == "PREPAID" {
		params["DiskChargePrepaid"] = map[string]interface{}{
			// Always price a single month: cloudtab reports a monthly run-rate
			// and the PREPAID DiscountPrice is a period total, so Period=1
			// keeps it monthly.
			"Period": 1,
		}
	}

	return pricing.PriceRequest{
		Product: "cbs",
		Action:  "InquiryPriceCreateDisks",
		Region:  r.Region,
		Params:  params,
	}, nil
}

// cbsDiskPrice is the nested disk price structure returned by the CBS
// InquiryPriceCreateDisks API.
type cbsDiskPrice struct {
	UnitPrice         float64 `json:"UnitPrice"`
	UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
	OriginalPrice     float64 `json:"OriginalPrice"`
	DiscountPrice     float64 `json:"DiscountPrice"`
	ChargeUnit        string  `json:"ChargeUnit"`
	Currency          string  `json:"Currency"`
}

func (CBSStorage) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// The Tencent Cloud SDK wraps real responses under a "Response" key.
	// Support both the wrapped format (real API) and the unwrapped format
	// (test mocks) for robustness.
	var wrap struct {
		DiskPrice cbsDiskPrice `json:"DiskPrice"`
		Response  struct {
			DiskPrice cbsDiskPrice `json:"DiskPrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	// Prefer the Response-wrapped price when it carries data.
	dp := wrap.DiskPrice
	if wrap.Response.DiskPrice.UnitPriceDiscount > 0 ||
		wrap.Response.DiskPrice.DiscountPrice > 0 ||
		wrap.Response.DiskPrice.UnitPrice > 0 {
		dp = wrap.Response.DiskPrice
	}

	currency := dp.Currency
	if currency == "" {
		currency = "CNY"
	}

	monthly, hourly := monthlyFromPrice(dp.ChargeUnit, dp.UnitPriceDiscount, dp.DiscountPrice)
	return []output.CostComponent{{
		Name:        fmt.Sprintf("CBS %v (%vGB)", req.Params["DiskType"], req.Params["DiskSize"]),
		Unit:        dp.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    currency,
	}}, nil
}
