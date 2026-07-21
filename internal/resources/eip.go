package resources

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// EIP handles `tencentcloud_eip`.
//
// Note: Tencent Cloud's VPC SDK does not expose an InquiryPrice* API for EIP.
// Real EIP cost depends on the charge type and, for traffic-based billing, on
// actual monthly traffic which is not present in a Terraform plan. We therefore
// return a zero-cost placeholder line with a descriptive note and route it
// through the StaticMapper path so it never calls the pricing engine.
type EIP struct{}

func (EIP) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{}, fmt.Errorf("EIP pricing is static; use Estimate")
}

func (EIP) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return nil, fmt.Errorf("EIP pricing is static; use Estimate")
}

func (EIP) Estimate(r parser.PlannedResource) ([]output.CostComponent, error) {
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

	chargeType := getStr("internet_charge_type")
	if chargeType == "" {
		chargeType = "BANDWIDTH_POSTPAID_BY_HOUR"
	}
	bw := getInt("internet_max_bandwidth_out")
	if bw == 0 {
		bw = 1
	}

	note := "price not available via Tencent InquiryPrice API; configure usage.yml"
	return []output.CostComponent{{
		Name:        fmt.Sprintf("EIP (%dMbps, %s) - %s", bw, chargeType, note),
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: 0,
		Currency:    "CNY",
	}}, nil
}
