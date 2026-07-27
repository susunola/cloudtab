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
// Docs: https://cloud.tencent.com/document/product/409/16777
//
// The API supports both PREPAID (包年包月) and POSTPAID (按量计费, 按小时).
// Note: the API enum is "POSTPAID" (NOT "POSTPAID_BY_HOUR"); the TF provider
// uses "POSTPAID_BY_HOUR", so we normalize it before sending.
//
// Response is a simplified top-level structure (not ItemPrice):
//   - OriginalPrice/Price are in 分 (cents); Price is the discounted total.
//   - PREPAID: Price is the period (Period=1 → monthly) total in 分.
//   - POSTPAID: Price is the hourly rate in 分.
//
// Terraform provider fields commonly seen:
// - availability_zone, spec_code, storage, instance_charge_type, prepaid_period
// - cpu, memory (used to derive SpecCode when spec_code is absent)
type PostgreSQLInstance struct{}

// pgSpec describes one sellable PostgreSQL "通用型" class.
type pgSpec struct {
	cpu, mem int64 // cpu cores, memory in GB
	spec     string
}

// pgSpecTable lists the general-purpose (pg.it.*) classes returned by
// DescribeClasses (stable across zones/versions). Used to translate the TF
// cpu+memory into a real SpecCode for InquiryPriceCreateDBInstances.
var pgSpecTable = []pgSpec{
	{1, 2, "pg.it.small2"},
	{2, 4, "pg.it.medium4"},
	{2, 6, "pg.it.medium6"},
	{4, 8, "pg.it.large8"},
	{4, 16, "pg.it.large16"},
	{6, 24, "pg.it.3xmedium24"},
	{8, 16, "pg.it.2xlarge16"},
	{8, 32, "pg.it.2xlarge32"},
	{8, 48, "pg.it.2xlarge48"},
	{8, 64, "pg.it.2xlarge64"},
	{12, 24, "pg.it.3xlarge24"},
	{12, 64, "pg.it.3xlarge64"},
	{16, 32, "pg.it.4xlarge32"},
	{16, 96, "pg.it.4xlarge96"},
	{20, 128, "pg.it.5xlarge128"},
	{24, 48, "pg.it.6xlarge48"},
	{24, 192, "pg.it.6xlarge192"},
	{28, 240, "pg.it.7xlarge240"},
	{32, 64, "pg.it.8xlarge64"},
	{32, 128, "pg.it.8xlarge128"},
	{48, 480, "pg.it.12xlarge480"},
	{64, 256, "pg.it.16xlarge256"},
	{64, 384, "pg.it.16xlarge384"},
	{64, 512, "pg.it.16xlarge512"},
	{90, 720, "pg.it.45xmedium720"},
	{128, 720, "pg.it.32xlarge720"},
	{156, 960, "pg.it.39xlarge960"},
}

// pgSpecCode resolves cpu+memory (GB) to a sellable SpecCode. When cpu is known
// it requires an exact match; otherwise it returns the smallest-cpu class for
// the given memory. Returns "" if no class matches (caller reports a clear err).
func pgSpecCode(cpu, memGB int64) string {
	if memGB <= 0 {
		return ""
	}
	if cpu > 0 {
		for _, s := range pgSpecTable {
			if s.cpu == cpu && s.mem == memGB {
				return s.spec
			}
		}
	}
	// cpu unknown/mismatch: pick the smallest-cpu class matching memory
	// (table is ordered by cpu, so the first memory match is the smallest).
	for _, s := range pgSpecTable {
		if s.mem == memGB {
			return s.spec
		}
	}
	return ""
}

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
	// The TF provider exposes cpu/memory (in GB) but not the pricing SpecCode.
	// Derive the real, sellable SpecCode (format pg.it.*, confirmed via
	// DescribeClasses) from cpu+memory. The legacy "cdb.pg.z1.{mem}g" format no
	// longer exists and triggers "参数Zone State检查失败" on InquiryPrice.
	if specCode == "" {
		specCode = pgSpecCode(getInt(r.After, "cpu"), getInt(r.After, "memory"))
	}
	if zone == "" || specCode == "" || storage == 0 {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_postgresql_instance requires availability_zone/spec_code( or memory)/storage")
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
		chargeType = "PREPAID" // API default
	}
	// The API enum is "PREPAID" or "POSTPAID". The TF provider uses
	// "POSTPAID_BY_HOUR" for pay-as-you-go; normalize it to "POSTPAID" so the
	// API does not reject the request with an invalid enum value.
	if strings.Contains(chargeType, "POSTPAID") && chargeType != "POSTPAID" {
		chargeType = "POSTPAID"
	}

	params := map[string]interface{}{
		"Zone":               zone,
		"SpecCode":           specCode,
		"Storage":            storage,
		"InstanceCount":      instanceCount,
		"InstanceChargeType": chargeType,
		// Always price a single month: cloudtab reports a monthly run-rate and
		// the PREPAID price is a period total, so Period=1 keeps it monthly.
		"Period": 1,
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

	// POSTPAID: Price is the hourly rate in 分; convert to 元/hour and ×730.
	// PREPAID: Price is already a monthly total (cloudtab forces Period=1).
	if chargeType == "POSTPAID" {
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
