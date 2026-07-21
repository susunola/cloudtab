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

	instanceType := getStr("instance_type")
	imageID := getStr("image_id")
	zone := getStr("availability_zone")
	if instanceType == "" || imageID == "" || zone == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_instance requires instance_type/image_id/availability_zone")
	}

	chargeType := getStr("instance_charge_type")
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
	if sd := getStr("system_disk_type"); sd != "" {
		params["SystemDisk"] = map[string]interface{}{
			"DiskType": sd,
			"DiskSize": getInt("system_disk_size"),
		}
	}
	if chargeType == "PREPAID" {
		params["InstanceChargePrepaid"] = map[string]interface{}{
			"Period":    getInt("instance_charge_type_prepaid_period"),
			"RenewFlag": getStr("instance_charge_type_prepaid_renew_flag"),
		}
	}

	return pricing.PriceRequest{
		Product: "cvm",
		Action:  "InquiryPriceRunInstances",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (CVMInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		Price struct {
			InstancePrice struct {
				UnitPrice         float64 `json:"UnitPrice"`
				UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
				OriginalPrice     float64 `json:"OriginalPrice"`
				DiscountPrice     float64 `json:"DiscountPrice"`
				ChargeUnit        string  `json:"ChargeUnit"`
			} `json:"InstancePrice"`
			BandwidthPrice struct {
				UnitPrice         float64 `json:"UnitPrice"`
				UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
				ChargeUnit        string  `json:"ChargeUnit"`
			} `json:"BandwidthPrice"`
		} `json:"Price"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	ip := wrap.Price.InstancePrice
	monthly := ip.UnitPriceDiscount * 730 // ~hours in a month; PREPAID returns fixed prices directly
	if ip.OriginalPrice > 0 {             // PREPAID path
		monthly = ip.DiscountPrice
	}
	comps := []output.CostComponent{{
		Name:         "Compute (" + req.Params["InstanceType"].(string) + ")",
		Unit:         ip.ChargeUnit,
		HourlyCost:   ip.UnitPriceDiscount,
		MonthlyCost:  monthly,
		Currency:     "CNY",
	}}
	if wrap.Price.BandwidthPrice.UnitPrice > 0 {
		comps = append(comps, output.CostComponent{
			Name:        "Public bandwidth",
			Unit:        wrap.Price.BandwidthPrice.ChargeUnit,
			HourlyCost:  wrap.Price.BandwidthPrice.UnitPriceDiscount,
			MonthlyCost: wrap.Price.BandwidthPrice.UnitPriceDiscount * 730,
			Currency:    "CNY",
		})
	}
	return comps, nil
}
