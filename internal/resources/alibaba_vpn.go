package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaVPN handles `alicloud_vpn_gateway`.
//
// Priced via Alibaba Cloud BSS GetPayAsYouGoPrice with ProductCode "vpn".
// ModuleList: Bandwidth.
type AlibabaVPN struct{}

func (AlibabaVPN) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	bandwidth := getInt(r.After, "bandwidth")
	if bandwidth <= 0 {
		bandwidth = 10
	}

	vpnType := strings.TrimSpace(getStr(r.After, "vpn_type"))
	if vpnType == "" {
		vpnType = "Normal"
	}
	// Extracted for reference; not used in the price request.
	_ = vpnType

	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "vpn",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList": []map[string]string{
				alibabaModule("Bandwidth", "Hour", fmt.Sprintf("%d:Mbps", bandwidth)),
			},
		},
	}, nil
}

func (AlibabaVPN) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba VPN Gateway",
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
