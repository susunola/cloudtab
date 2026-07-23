package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiEIP handles `huaweicloud_vpc_eip` (elastic IP / bandwidth).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings. An EIP is composed
// of two billable parts, both modeled as separate product_infos:
//
//  1. The public IP itself  -> cloud_service_type "hws.service.type.vpc",
//     resource_type "hws.resource.type.ip", resource_spec is the EIP line type
//     (e.g. "5_bgp" dynamic BGP, "5_sbgp" static BGP). Billed by Duration.
//  2. The bandwidth attachment -> resource_type "hws.resource.type.bandwidth".
//     Bandwidth is a LINEAR product, so resource_size (Mbps) + size_measure_id
//     (15 = Mbps) are mandatory. It is billed by Duration (by-bandwidth) or by
//     upflow (by-traffic) depending on the plan's charge_mode.
type HuaweiEIP struct{}

func (HuaweiEIP) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	eipType := strings.TrimSpace(getStr(r.After, "type"))
	if eipType == "" {
		eipType = "5_bgp"
	}

	// Bandwidth block (huaweicloud_vpc_eip.bandwidth). In a real Terraform plan
	// this is a nested block: either a single object or a one-element list, which
	// getNestedMap tolerates. Only the direct getStr lookup would fail on it.
	bw := getNestedMap(r.After, "bandwidth")
	chargeMode := strings.ToLower(strings.TrimSpace(getStr(bw, "charge_mode")))
	if chargeMode == "" {
		chargeMode = strings.ToLower(strings.TrimSpace(getStr(r.After, "charging_mode")))
	}
	bwSize := getInt(bw, "size")
	if bwSize <= 0 {
		bwSize = 5
	}

	// Part 1: the public IP (billed by Duration, per hour).
	productInfos := []map[string]interface{}{
		huaweiProductInfo("hws.service.type.vpc", "hws.resource.type.ip", eipType, r.Region, 0, 0),
	}

	// Part 2: the bandwidth (linear). By-traffic -> upflow (per-GB, measure 10);
	// by-bandwidth (default) -> Duration (per hour, measure 4). resource_size is
	// in Mbps and size_measure_id 15 = Mbps.
	if chargeMode == "traffic" {
		productInfos = append(productInfos,
			huaweiProductInfoEx("hws.service.type.vpc", "hws.resource.type.bandwidth", eipType, r.Region, int(bwSize), 15, "upflow", 10))
	} else {
		productInfos = append(productInfos,
			huaweiProductInfo("hws.service.type.vpc", "hws.resource.type.bandwidth", eipType, r.Region, int(bwSize), 15))
	}

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "eip",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": productInfos,
		},
	}, nil
}

func (HuaweiEIP) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei EIP",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
