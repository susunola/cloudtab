package resources

import (
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AlibabaECS handles `alicloud_instance` (ECS).
//
// Priced via Alibaba Cloud BSS GetPayAsYouGoPrice with ProductCode "ecs".
// ModuleList: InstanceType, SystemDisk (optional).
type AlibabaECS struct{}

func (AlibabaECS) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := strings.TrimSpace(getStr(r.After, "instance_type"))
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("alicloud_instance requires instance_type")
	}

	// Detect OS from image_id: if it contains "win" → windows, else linux.
	osType := "linux"
	if img := strings.TrimSpace(getStr(r.After, "image_id")); strings.Contains(strings.ToLower(img), "win") {
		osType = "windows"
	}

	moduleList := []map[string]string{
		alibabaModule("InstanceType", "Hour", "InstanceType:"+instanceType+",ImageOs:"+osType),
	}

	// System disk
	if cat := strings.TrimSpace(getStr(r.After, "system_disk_category")); cat != "" {
		size := getInt(r.After, "system_disk_size")
		if size <= 0 {
			size = 40
		}
		moduleList = append(moduleList, alibabaModule("SystemDisk", "Hour", fmt.Sprintf("SystemDisk.Category:%s,SystemDisk.Size:%d", cat, size)))
	}

	return pricing.PriceRequest{
		Provider: "alibaba",
		Product:  "ecs",
		Region:   r.Region,
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList":       moduleList,
		},
	}, nil
}

func (AlibabaECS) Parse(_ pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	info, err := parseAlibabaPrice(raw)
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "Alibaba ECS",
		Unit:        "HOUR",
		HourlyCost:  info.PriceYuan,
		MonthlyCost: info.PriceYuan * hoursPerMonth,
		Currency:    info.Currency,
	}}, nil
}
