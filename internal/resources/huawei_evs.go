package resources

import (
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// HuaweiEVS handles `huaweicloud_evs_volume` (EVS disk).
//
// Priced via Huawei Cloud BSS ListOnDemandResourceRatings.
type HuaweiEVS struct{}

func (HuaweiEVS) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	volumeType := strings.TrimSpace(getStr(r.After, "volume_type"))
	if volumeType == "" {
		volumeType = "SAS"
	}
	size := getInt(r.After, "size")
	if size <= 0 {
		size = 40
	}

	return pricing.PriceRequest{
		Provider: "huawei",
		Product:  "evs",
		Region:   r.Region,
		Params: map[string]interface{}{
			"product_infos": []map[string]interface{}{
				// EVS is billed per-GB (linear), so include resource_size and
				// size_measure_id (17 = GB). usage_factor stays "Duration".
				huaweiProductInfo("hws.service.type.ebs", "hws.resource.type.volume", volumeType, r.Region, int(size), 17),
			},
		},
	}, nil
}

func (HuaweiEVS) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseHuaweiPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Huawei EVS",
		Unit:        "HOUR",
		HourlyCost:  info.Amount,
		MonthlyCost: info.Amount * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
