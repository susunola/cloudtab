package resources

import (
	"encoding/json"
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// CVMInstance handles `tencentcloud_instance`.
//
// Reference: https://cloud.tencent.com/document/api/213/15726
// (InquiryPriceRunInstances → Response.Price.InstancePrice + BandwidthPrice)
type CVMInstance struct{}

func (CVMInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := getStr(r.After, "instance_type")
	imageID := getStr(r.After, "image_id")
	zone := getStr(r.After, "availability_zone")
	if instanceType == "" || imageID == "" || zone == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_instance requires instance_type/image_id/availability_zone")
	}

	chargeType := getStr(r.After, "instance_charge_type")
	if chargeType == "" {
		chargeType = "POSTPAID_BY_HOUR"
	}

	params := map[string]interface{}{
		"Placement":          map[string]interface{}{"Zone": zone},
		"ImageId":            imageID,
		"InstanceType":       instanceType,
		"InstanceChargeType": chargeType,
		"InstanceCount":      1,
	}
	if sd := getStr(r.After, "system_disk_type"); sd != "" {
		params["SystemDisk"] = map[string]interface{}{
			"DiskType": sd,
			"DiskSize": getInt(r.After, "system_disk_size"),
		}
	}
	if chargeType == "PREPAID" {
		params["InstanceChargePrepaid"] = map[string]interface{}{
			// Always price a single month: cloudtab reports a monthly run-rate
			// and the PREPAID DiscountPrice is a period total, so Period=1
			// keeps it monthly.
			"Period":    1,
			"RenewFlag": getStr(r.After, "instance_charge_type_prepaid_renew_flag"),
		}
	}

	return pricing.PriceRequest{
		Product: "cvm",
		Action:  "InquiryPriceRunInstances",
		Region:  r.Region,
		Params:  params,
	}, nil
}

// cvmPriceBlock is the nested price structure returned by the CVM
// InquiryPriceRunInstances API.
type cvmPriceBlock struct {
	InstancePrice struct {
		UnitPrice         float64 `json:"UnitPrice"`
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		OriginalPrice     float64 `json:"OriginalPrice"`
		DiscountPrice     float64 `json:"DiscountPrice"`
		ChargeUnit        string  `json:"ChargeUnit"`
		Currency          string  `json:"Currency"`
	} `json:"InstancePrice"`
	BandwidthPrice struct {
		UnitPrice         float64 `json:"UnitPrice"`
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		ChargeUnit        string  `json:"ChargeUnit"`
	} `json:"BandwidthPrice"`
}

func (CVMInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// The Tencent Cloud SDK wraps real responses under a "Response" key.
	// Support both the wrapped format (real API) and the unwrapped format
	// (test mocks) for robustness.
	var wrap struct {
		Price    cvmPriceBlock `json:"Price"`
		Response struct {
			Price cvmPriceBlock `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	// Prefer the Response-wrapped price when it carries data.
	price := wrap.Price
	if wrap.Response.Price.InstancePrice.UnitPriceDiscount > 0 ||
		wrap.Response.Price.InstancePrice.DiscountPrice > 0 ||
		wrap.Response.Price.InstancePrice.UnitPrice > 0 {
		price = wrap.Response.Price
	}

	currency := price.InstancePrice.Currency
	if currency == "" {
		currency = "CNY"
	}

	ip := price.InstancePrice
	monthly, hourly := monthlyFromPrice(ip.ChargeUnit, ip.UnitPriceDiscount, ip.DiscountPrice)
	comps := []output.CostComponent{{
		Name:        fmt.Sprintf("Compute (%v)", req.Params["InstanceType"]),
		Unit:        ip.ChargeUnit,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    currency,
	}}
	if price.BandwidthPrice.UnitPrice > 0 {
		bw := price.BandwidthPrice
		bwMonthly, bwHourly := monthlyFromPrice(bw.ChargeUnit, bw.UnitPriceDiscount, 0)
		comps = append(comps, output.CostComponent{
			Name:        "Public bandwidth",
			Unit:        bw.ChargeUnit,
			HourlyCost:  bwHourly,
			MonthlyCost: bwMonthly,
			Currency:    currency,
		})
	}
	return comps, nil
}
