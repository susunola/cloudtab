package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// PostgreSQLInstance handles `tencentcloud_postgresql_instance`.
//
// Pricing API (postgres): InquiryPriceCreateDBInstances.
// Docs: https://cloud.tencent.com/document/api/409/16777
//
// Terraform provider fields commonly seen:
// - availability_zone, spec_code, storage, instance_charge_type, prepaid_period
// - cpu, memory
type PostgreSQLInstance struct{}

func (PostgreSQLInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zone := strings.TrimSpace(getStr(r.After, "availability_zone"))
	if zone == "" {
		zone = getStr(r.After, "zone")
	}
	specCode := strings.TrimSpace(getStr(r.After, "spec_code"))
	storage := getInt(r.After, "storage")
	if storage == 0 {
		storage = getInt(r.After, "volume")
	}
	if zone == "" || specCode == "" || storage == 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_postgresql_instance requires availability_zone/spec_code/storage")
	}

	instanceCount := getInt(r.After, "count")
	if instanceCount <= 0 {
		instanceCount = 1
	}

	chargeType := strings.ToUpper(getStr(r.After, "instance_charge_type"))
	if chargeType == "" {
		chargeType = strings.ToUpper(getStr(r.After, "charge_type"))
	}
	if chargeType == "" {
		chargeType = "POSTPAID_BY_HOUR"
	}

	period := getInt(r.After, "prepaid_period")
	if period <= 0 {
		period = getInt(r.After, "period")
	}
	if period <= 0 {
		period = 1
	}

	params := map[string]interface{}{
		"Zone":               zone,
		"SpecCode":           specCode,
		"Storage":            storage,
		"InstanceCount":      instanceCount,
		"InstanceChargeType": chargeType,
		"Period":             period,
	}

	return pricing.PriceRequest{
		Product: "postgres",
		Action:  "InquiryPriceCreateDBInstances",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (PostgreSQLInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	p, err := parseTencentPrice(raw)
	if err != nil {
		return nil, err
	}

	priceYuan := preferDiscount(p.Price, p.Original) / 100.0
	chargeType := fmt.Sprintf("%v", req.Params["InstanceChargeType"])
	monthly := priceYuan
	hourly := 0.0

	// POSTPAID_BY_HOUR indicates hourly billing.
	if strings.Contains(chargeType, "HOUR") || chargeType == "POSTPAID" {
		hourly = priceYuan
		monthly = priceYuan * hoursPerMonth
	}

	return []output.CostComponent{{
		Name:        fmt.Sprintf("PostgreSQL (spec %v, %vGB)", req.Params["SpecCode"], req.Params["Storage"]),
		Unit:        chargeType,
		HourlyCost:  hourly,
		MonthlyCost: monthly,
		Currency:    p.Currency,
	}}, nil
}
