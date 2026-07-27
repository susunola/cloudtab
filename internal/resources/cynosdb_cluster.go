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
// Docs: https://cloud.tencent.com/document/api/1003/77738
//
// The API supports three instance pay modes:
//   - PREPAID: requires Cpu, Memory, TimeSpan, TimeUnit
//   - POSTPAID: requires Cpu, Memory
//   - SERVERLESS: requires Ccu (no Cpu/Memory)
//
// The Terraform provider (>=1.83) uses different field names:
//   - charge_type: PREPAID | POSTPAID_BY_HOUR (NOT "POSTPAID")
//   - db_mode: NORMAL (default) | SERVERLESS
//   - NORMAL mode: instance_cpu_core + instance_memory_size
//   - SERVERLESS mode: min_cpu + max_cpu (NOT "ccu" — that's an API-only field)
//   - storage_pay_mode: 0 (pay-as-you-go) | 1 (prepaid), as number
//
// We normalize the TF values to the API's expected pay modes (PREPAID/POSTPAID/
// SERVERLESS) and map min/max CPU to Ccu for the API call.
//
// Storage pay mode is independent of instance pay mode (PREPAID or POSTPAID);
// PREPAID storage additionally requires StorageLimit.
//
// Response has two TradePrice blocks — InstancePrice and StoragePrice — each in
// cents (Number). PREPAID uses TotalPriceDiscount (period total); POSTPAID uses
// UnitPriceDiscount (hourly). Storage POSTPAID ChargeUnit is "GB*h" (per GB per
// hour). We sum instance + storage.
type CynosDBCluster struct{}

func (CynosDBCluster) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	zone := strings.TrimSpace(getStr(r.After, "available_zone"))
	if zone == "" {
		zone = getStr(r.After, "availability_zone")
	}
	if zone == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_cynosdb_cluster requires available_zone")
	}

	// TF provider uses db_mode (NORMAL | SERVERLESS) to distinguish instance type.
	dbMode := strings.ToUpper(strings.TrimSpace(getStr(r.After, "db_mode")))

	// CPU/Memory: NORMAL mode uses instance_cpu_core/instance_memory_size (or
	// cpu/memory as fallback); SERVERLESS mode uses min_cpu/max_cpu.
	cpu := getInt(r.After, "cpu")
	if cpu == 0 {
		cpu = getInt(r.After, "instance_cpu_core")
	}
	memory := getInt(r.After, "memory")
	if memory == 0 {
		memory = getInt(r.After, "instance_memory_size")
	}
	maxCPU := getInt(r.After, "max_cpu")

	goodsNum := getInt(r.After, "instance_count")
	if goodsNum <= 0 {
		goodsNum = 1
	}
	storage := getInt(r.After, "storage_limit")
	if storage <= 0 {
		storage = getInt(r.After, "storage")
	}

	// Determine instance pay mode. The API accepts PREPAID | POSTPAID | SERVERLESS.
	// TF charge_type uses PREPAID | POSTPAID_BY_HOUR; db_mode SERVERLESS overrides.
	chargeType := strings.ToUpper(strings.TrimSpace(getStr(r.After, "charge_type")))
	if chargeType == "" {
		chargeType = strings.ToUpper(strings.TrimSpace(getStr(r.After, "instance_charge_type")))
	}

	instancePayMode := "POSTPAID"
	switch {
	case dbMode == "SERVERLESS":
		instancePayMode = "SERVERLESS"
	case strings.HasPrefix(chargeType, "PREPAID"):
		instancePayMode = "PREPAID"
	}

	// Validate required parameters per pay mode.
	if instancePayMode == "SERVERLESS" {
		if maxCPU <= 0 {
			return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_cynosdb_cluster SERVERLESS requires max_cpu")
		}
	} else {
		// PREPAID and POSTPAID both require Cpu + Memory.
		if cpu <= 0 || memory <= 0 {
			return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_cynosdb_cluster %s requires cpu/memory", instancePayMode)
		}
	}

	// Storage pay mode is independent of instance pay mode.
	// TF storage_pay_mode: 0=POSTPAID, 1=PREPAID. If absent, infer from
	// charge_type (PREPAID → storage PREPAID, else POSTPAID).
	storagePayMode := "POSTPAID"
	if spm := getInt(r.After, "storage_pay_mode"); spm == 1 {
		storagePayMode = "PREPAID"
	} else if spm == 0 && strings.HasPrefix(chargeType, "PREPAID") {
		storagePayMode = "PREPAID"
	}
	if storagePayMode == "PREPAID" && storage <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_cynosdb_cluster PREPAID storage requires storage_limit")
	}

	params := map[string]interface{}{
		"Zone":            zone,
		"GoodsNum":        goodsNum,
		"InstancePayMode": instancePayMode,
		"StoragePayMode":  storagePayMode,
	}
	if instancePayMode == "SERVERLESS" {
		// API uses Ccu; derive from TF's max_cpu (CCU ≈ max CPU cores).
		params["Ccu"] = maxCPU
	} else {
		params["Cpu"] = cpu
		params["Memory"] = memory
	}
	if storage > 0 {
		params["StorageLimit"] = storage
	}
	if dt := getStr(r.After, "device_type"); dt != "" {
		params["DeviceType"] = dt
	}

	if instancePayMode == "PREPAID" {
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

// cynosTradePrice mirrors cynosdb TradePrice (amounts in cents, Number).
// https://cloud.tencent.com/document/api/1003/77738#TradePrice
type cynosTradePrice struct {
	TotalPrice         float64 `json:"TotalPrice"`
	TotalPriceDiscount float64 `json:"TotalPriceDiscount"`
	UnitPrice          float64 `json:"UnitPrice"`
	UnitPriceDiscount  float64 `json:"UnitPriceDiscount"`
	ChargeUnit         string  `json:"ChargeUnit"`
	Discount           float64 `json:"Discount"`
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

	instancePayMode := strings.ToUpper(fmt.Sprintf("%v", req.Params["InstancePayMode"]))
	storagePayMode := strings.ToUpper(fmt.Sprintf("%v", req.Params["StoragePayMode"]))

	comps := make([]output.CostComponent, 0, 2)
	comps = append(comps, cynosComponent("TDSQL-C compute", pb.InstancePrice,
		strings.HasPrefix(instancePayMode, "PREPAID")))
	if pb.StoragePrice.hasPrice() {
		comps = append(comps, cynosComponent("TDSQL-C storage", pb.StoragePrice,
			strings.HasPrefix(storagePayMode, "PREPAID")))
	}
	return comps, nil
}

// cynosComponent converts one TradePrice block (cents) into a CostComponent.
// For POSTPAID the discounted unit price is an hourly rate; for PREPAID the
// discounted total is a period total treated as the monthly figure.
// Storage POSTPAID uses ChargeUnit "GB*h" (per GB per hour); the unit price is
// per-GB-per-hour, so without storage volume we can only show the rate.
func cynosComponent(name string, tp cynosTradePrice, prepaid bool) output.CostComponent {
	if prepaid {
		total := tp.TotalPriceDiscount
		if total == 0 {
			total = tp.TotalPrice
		}
		return output.CostComponent{
			Name:        name,
			Unit:        "MONTH",
			HourlyCost:  0,
			MonthlyCost: total / 100.0,
			Currency:    "CNY",
		}
	}
	// POSTPAID: UnitPriceDiscount is the hourly rate (cents).
	unit := tp.UnitPriceDiscount
	if unit == 0 {
		unit = tp.UnitPrice
	}
	chargeUnit := strings.ToUpper(strings.TrimSpace(tp.ChargeUnit))
	if chargeUnit == "GB*H" {
		// Storage per-GB-per-hour: without storage volume we can only show the
		// rate (CNY/GB-h). Monthly cost depends on StorageLimit x usage hours.
		rate := unit / 100.0
		return output.CostComponent{
			Name:        fmt.Sprintf("%s (%.4f CNY/GB-h)", name, rate),
			Unit:        "GB*H",
			HourlyCost:  0,
			MonthlyCost: 0,
			Currency:    "CNY",
		}
	}
	hourly := unit / 100.0
	return output.CostComponent{
		Name:        name,
		Unit:        "HOUR",
		HourlyCost:  hourly,
		MonthlyCost: hourly * hoursPerMonth,
		Currency:    "CNY",
	}
}
