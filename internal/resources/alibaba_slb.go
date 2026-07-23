package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaSLB handles `alicloud_slb_load_balancer` (Server Load Balancer, CLB).
//
// Priced via Alibaba Cloud BSS GetPayAsYouGoPrice with ProductCode "slb".
// ModuleList (fixed-bandwidth billing): LoadBalancerSpec, InternetTrafficOut
// (0 = fixed bandwidth), InstanceRent, Bandwidth (Kbps).
type AlibabaSLB struct{}

func (AlibabaSLB) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	spec := strings.TrimSpace(getStr(r.After, "specification"))
	if spec == "" {
		spec = "slb.s1.small"
	}
	// SLB bandwidth is expressed in Mbps in the Terraform plan; the BSS API
	// expects Kbps (1024..204800, multiple of 1024).
	bwKbps := getInt(r.After, "bandwidth") * 1024
	if bwKbps <= 0 {
		bwKbps = 1024
	}

	moduleList := []map[string]string{
		alibabaModule("LoadBalancerSpec", "Hour", "LoadBalancerSpec:"+spec),
		alibabaModule("InternetTrafficOut", "Usage", "InternetTrafficOut:0"),
		alibabaModule("InstanceRent", "Hour", "InstanceRent:1"),
		alibabaModule("Bandwidth", "Hour", fmt.Sprintf("Bandwidth:%d", bwKbps)),
	}

	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "slb",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList":       moduleList,
		},
	}, nil
}

func (AlibabaSLB) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        fmt.Sprintf("Alibaba SLB"),
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
