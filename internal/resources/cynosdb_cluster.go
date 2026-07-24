package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// CynosDBCluster handles `tencentcloud_cynosdb_cluster` (TDSQL-C).
//
// Pricing API (cynosdb): InquirePriceCreate (SDK spells it "InquirePrice",
// without the 'y').
// Docs: https://cloud.tencent.com/document/api/1003/48086
//
// Terraform provider fields commonly seen:
//   - available_zone, cpu, memory (GB), storage_limit (GB),
//     charge_type (PREPAID | POSTPAID), prepaid_period, instance_count
//
// Response has two TradePrice blocks — InstancePrice and StoragePrice — each in
// cents (int64). PREPAID uses TotalPriceDiscount (period total); POSTPAID uses
// UnitPriceDiscount (hourly). We sum instance + storage.
type CynosDBCluster struct{}

func (CynosDBCluster) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zone := strings.TrimSpace(getStr(r.After, "available_zone"))
	if zone == "" {
		zone = getStr(r.After, "availability_zone")
	}
	cpu := getInt(r.After, "cpu")
	memory := getInt(r.After, "memory")
	if zone == "" || cpu <= 0 || memory <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_cynosdb_cluster requires available_zone/cpu/memory")
	}

	goodsNum := getInt(r.After, "instance_count")
	if goodsNum <= 0 {
		goodsNum = 1
	}
	storage := getInt(r.After, "storage_limit")
	if storage <= 0 {
		storage = getInt(r.After, "storage")
	}

	chargeType := strings.ToUpper(strings.TrimSpace(getStr(r.After, "charge_type")))
	if chargeType == "" {
		chargeType = strings.ToUpper(strings.TrimSpace(getStr(r.After, "instance_charge_type")))
	}
	// cynosdb pay mode expects "PREPAID" | "POSTPAID".
	payMode := "POSTPAID"
	if strings.HasPrefix(chargeType, "PREPAID") {
		payMode = "PREPAID"
	}

	params := map[string]interface{}{
		"Zone":            zone,
		"GoodsNum":        goodsNum,
		"InstancePayMode": payMode,
		"StoragePayMode":  payMode,
		"Cpu":             cpu,
		"Memory":          memory,
	}
	if storage > 0 {
		params["StorageLimit"] = storage
	}
	if dt := getStr(r.After, "device_type"); dt != "" {
		params["DeviceType"] = dt
	}

	if payMode == "PREPAID" {
		// Price a single month: cloudtab reports a monthly run-rate, and the
		// PREPAID TotalPriceDiscount is a period total. TimeSpan=1 month keeps
		// the returned total equal to the monthly cost by construction.
		params["TimeSpan"] = 1
		params["TimeUnit"] = "m" // months
	}

	return pricing.PriceRequest{
		Product: "cynosdb",
		Action:  "InquirePriceCreate",
		Region:  r.Region,
		Params:  params,
	}, nil
}

// cynosTradePrice mirrors cynosdb TradePrice (all amounts in cents, int64).
type cynosTradePrice struct {
	TotalPrice         int64 `json:"TotalPrice"`
	TotalPriceDiscount int64 `json:"TotalPriceDiscount"`
	UnitPrice          int64 `json:"UnitPrice"`
	UnitPriceDiscount  int64 `json:"UnitPriceDiscount"`
}

func (tp cynosTradePrice) hasPrice() bool {
	return tp.TotalPriceDiscount > 0 || tp.UnitPriceDiscount > 0 ||
		tp.TotalPrice > 0 || tp.UnitPrice > 0
}

type cynosPriceBlock struct {
	InstancePrice cynosTradePrice `json:"InstancePrice"`
	StoragePrice  cynosTradePrice `json:"StoragePrice"`
}

func (CynosDBCluster) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	var wrap struct {
		cynosPriceBlock
		Response cynosPriceBlock `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	pb := wrap.cynosPriceBlock
	// The Tencent SDK nests the real payload under "Response"; prefer it.
	if wrap.Response.InstancePrice.hasPrice() || wrap.Response.StoragePrice.hasPrice() {
		pb = wrap.Response
	}

	payMode := strings.ToUpper(fmt.Sprintf("%v", req.Params["InstancePayMode"]))
	postpaid := !strings.HasPrefix(payMode, "PREPAID")

	comps := make([]output.CostComponent, 0, 2)
	comps = append(comps, cynosComponent("TDSQL-C compute", pb.InstancePrice, postpaid))
	if pb.StoragePrice.hasPrice() {
		comps = append(comps, cynosComponent("TDSQL-C storage", pb.StoragePrice, postpaid))
	}
	return comps, nil
}

// cynosComponent converts one TradePrice block (cents) into a CostComponent.
// For POSTPAID the discounted unit price is an hourly rate; for PREPAID the
// discounted total is a period total treated as the monthly figure.
func cynosComponent(name string, tp cynosTradePrice, postpaid bool) output.CostComponent {
	if postpaid {
		unit := tp.UnitPriceDiscount
		if unit == 0 {
			unit = tp.UnitPrice
		}
		hourly := float64(unit) / 100.0
		return output.CostComponent{
			Name:        name,
			Unit:        "HOUR",
			HourlyCost:  hourly,
			MonthlyCost: hourly * hoursPerMonth,
			Currency:    "CNY",
		}
	}
	total := tp.TotalPriceDiscount
	if total == 0 {
		total = tp.TotalPrice
	}
	return output.CostComponent{
		Name:        name,
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: float64(total) / 100.0,
		Currency:    "CNY",
	}
}
