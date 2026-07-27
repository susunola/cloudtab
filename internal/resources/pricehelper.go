package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
)

// hoursPerMonth is the conventional Tencent Cloud billing month (30.4 days).
// Tencent's own console uses ~730h for POSTPAID monthly estimates.
const hoursPerMonth = 730.0

// simpleHourlyCost wraps a single hourly rate into the one-component slice
// most Huawei/Alibaba mappers return. Monthly is derived as hourly × hoursPerMonth.
func simpleHourlyCost(name string, hourly float64, currency string) []output.CostComponent {
	return []output.CostComponent{{
		Name:        name,
		Unit:        "HOUR",
		HourlyCost:  hourly,
		MonthlyCost: hourly * hoursPerMonth,
		Currency:    currency,
	}}
}

// monthlyFromPrice converts an InquiryPrice* discounted price into a monthly
// figure, deciding PREPAID vs POSTPAID from the official ChargeUnit field
// rather than guessing from OriginalPrice.
//
//   - ChargeUnit == "HOUR"  → POSTPAID: unitPriceDiscount is CNY/hour, ×730 for monthly.
//   - ChargeUnit == "MONTH" → PREPAID:  discountPrice is already the monthly total.
//   - other / empty         → fall back to the POSTPAID hourly assumption; if
//     discountPrice is set and unitPriceDiscount is 0 we treat it as a fixed price.
//
// Returns (monthlyCost, hourlyCost). hourlyCost is 0 for non-hourly billing.
func monthlyFromPrice(chargeUnit string, unitPriceDiscount, discountPrice float64) (monthly, hourly float64) {
	switch strings.ToUpper(strings.TrimSpace(chargeUnit)) {
	case "HOUR":
		return unitPriceDiscount * hoursPerMonth, unitPriceDiscount
	case "MONTH", "NONE":
		return discountPrice, 0
	case "DAY":
		return unitPriceDiscount * (hoursPerMonth / 24), 0 // rare; day-rate × ~30.4 days
	default:
		// Unknown unit. Prefer an explicit fixed price if present,
		// otherwise assume the value is an hourly rate.
		if unitPriceDiscount > 0 {
			return unitPriceDiscount * hoursPerMonth, unitPriceDiscount
		}
		return discountPrice, 0
	}
}

// discountedYuanFromCents resolves the standard cents-based (cents) DescribePrice
// response used by the mariadb / sqlserver / dcdb mappers, which all return
// int64 Price/OriginalPrice both at the top level AND under a nested "Response"
// wrapper. It encodes the three identical decisions those mappers previously
// duplicated verbatim:
//
//  1. Dual-path: prefer the nested Response pair when it carries data (real SDK
//     ToJsonString output), else use the top-level pair (test mocks).
//  2. Discount fallback: prefer the discounted Price; fall back to OriginalPrice
//     when the API returned no discount (Price == 0).
//  3. Unit: divide cents by 100 to get CNY.
//
// Returns the resolved price in CNY.
func discountedYuanFromCents(topPrice, topOrig, respPrice, respOrig int64) float64 {
	price, orig := topPrice, topOrig
	if respPrice > 0 || respOrig > 0 {
		price, orig = respPrice, respOrig
	}
	if price == 0 {
		price = orig
	}
	return float64(price) / 100.0
}

// preferDiscount returns the discounted price when the API populated it
// (discount > 0) and falls back to the undiscounted original otherwise. The
// CNY-based mappers (lighthouse / ecm / gaap) share this "prefer discount, fall
// back to original" rule; only the surrounding struct shape differs, so each
// caller selects its own (discount, original) pair first and then applies this.
func preferDiscount(discount, original float64) float64 {
	if discount > 0 {
		return discount
	}
	return original
}

// splitByBilling maps a single per-unit CNY price onto cloudtab's (monthly,
// hourly) convention for the DescribePrice-style DB APIs (mariadb / sqlserver /
// dcdb), whose PREPAID call returns a monthly total (Period forced to 1) while
// the POSTPAID call returns an hourly rate.
//
// The caller decides postpaid vs prepaid because each API names the charge-type
// field differently (Paymode "postpaid" vs InstanceChargeType != "PREPAID"),
// but the arithmetic afterwards is identical:
//   - prepaid:  the price IS the monthly cost; hourly is 0.
//   - postpaid: the price is hourly; monthly = hourly × 730.
func splitByBilling(priceYuan float64, postpaid bool) (monthly, hourly float64) {
	if postpaid {
		return priceYuan * hoursPerMonth, priceYuan
	}
	return priceYuan, 0
}

// tencentSimplePrice is the common price shape returned by many Tencent Cloud
// DescribePrice / InquiryPrice* APIs: a discounted Price and an OriginalPrice,
// both in cents, plus a Currency string.
type tencentSimplePrice struct {
	Price    float64 `json:"Price"`
	Original float64 `json:"OriginalPrice"`
	Currency string  `json:"Currency"`
}

// parseTencentPrice unmarshals a raw Tencent Cloud pricing response into a
// tencentSimplePrice. The Tencent SDK wraps the real payload under a "Response"
// key; this helper prefers the nested version when populated and falls back to
// the top-level fields (used by test mocks). Currency defaults to "CNY" when
// absent.
func parseTencentPrice(raw []byte) (tencentSimplePrice, error) {
	var wrap struct {
		tencentSimplePrice
		Response tencentSimplePrice `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return tencentSimplePrice{}, err
	}
	p := wrap.tencentSimplePrice
	if wrap.Response.Price > 0 {
		p.Price = wrap.Response.Price
	}
	if wrap.Response.Original > 0 {
		p.Original = wrap.Response.Original
	}
	if wrap.Response.Currency != "" {
		p.Currency = wrap.Response.Currency
	}
	if p.Currency == "" {
		p.Currency = "CNY"
	}
	return p, nil
}

// --- Alibaba Cloud helpers ---

// alibabaPriceInfo is the common BSS GetPayAsYouGoPrice response price shape.
type alibabaPriceInfo struct {
	PriceYuan float64 // sum of ModuleDetail[].CostAfterDiscount (or OriginalCost fallback)
	Currency  string
}

// alibabaModulePrice carries a single module's resolved cost and its module code.
type alibabaModulePrice struct {
	ModuleCode string
	CostYuan   float64
}

// parseAlibabaPrice unmarshals a raw Alibaba Cloud BSS GetPayAsYouGoPrice
// response into a simplified price info struct. It sums CostAfterDiscount
// across all module details, falling back to OriginalCost. When the response
// omits its currency field, expectedCurrency is used (intl Alibaba quotes in
// USD) so the price is labelled correctly instead of silently assumed CNY; an
// explicit API currency always wins over expectedCurrency.
//
// A response that yields no positive price (empty ModuleDetails, or a
// business-level failure returned with HTTP 200) is an ERROR, not a zero-cost
// component: fabricating a confident "free" for a paid resource is exactly the
// anti-fabrication hazard the AWS truncation guard also protects against. The
// engine surfaces this as a skipped resource with a note rather than a bogus 0.
func parseAlibabaPrice(raw []byte, expectedCurrency string) (alibabaPriceInfo, error) {
	modules, currency, err := parseAlibabaModules(raw, expectedCurrency)
	if err != nil {
		return alibabaPriceInfo{}, err
	}
	var total float64
	for _, m := range modules {
		total += m.CostYuan
	}
	if total <= 0 {
		return alibabaPriceInfo{}, fmt.Errorf("alibaba: price response carried no positive cost (empty ModuleDetails or a business-level failure); refusing to report a fabricated zero")
	}
	return alibabaPriceInfo{PriceYuan: total, Currency: currency}, nil
}

// parseAlibabaModules unmarshals the BSS response and returns each module's
// code and discounted cost. This lets mappers with mixed PriceType modules
// (e.g. EIP bandwidth per-day + ISP per-hour) convert each component with the
// correct unit instead of collapsing everything into a single daily rate.
// expectedCurrency is used only when the response omits its own Currency field
// (see parseAlibabaPrice).
func parseAlibabaModules(raw []byte, expectedCurrency string) ([]alibabaModulePrice, string, error) {
	var resp struct {
		Data struct {
			Currency      string `json:"Currency"`
			ModuleDetails struct {
				ModuleDetail []struct {
					ModuleCode        string  `json:"ModuleCode"`
					CostAfterDiscount float64 `json:"CostAfterDiscount"`
					OriginalCost      float64 `json:"OriginalCost"`
				} `json:"ModuleDetail"`
			} `json:"ModuleDetails"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, "", err
	}
	cur := resp.Data.Currency
	if cur == "" {
		if expectedCurrency != "" {
			cur = expectedCurrency
		} else {
			cur = "CNY"
		}
	}
	var modules []alibabaModulePrice
	for _, md := range resp.Data.ModuleDetails.ModuleDetail {
		v := md.CostAfterDiscount
		if v <= 0 {
			v = md.OriginalCost
		}
		modules = append(modules, alibabaModulePrice{ModuleCode: md.ModuleCode, CostYuan: v})
	}
	return modules, cur, nil
}

// --- Huawei Cloud helpers ---

// huaweiPriceInfo is the common BSS ListOnDemandResourceRatings response shape.
type huaweiPriceInfo struct {
	Amount   float64 // amount in CNY (or USD for international)
	Currency string
}

// parseHuaweiPrice unmarshals a raw Huawei Cloud BSS ListOnDemandResourceRatings
// response. It reads the top-level "amount" field, falling back to
// "official_website_amount". An explicit API currency always wins; when the
// response omits it, expectedCurrency is used (intl Huawei quotes in USD) so
// the price is labelled correctly instead of silently assumed CNY.
//
// A response that yields no positive amount (empty body, or a business-level
// failure returned with HTTP 200) is an ERROR, not a zero-cost component — the
// same anti-fabrication guard applied to parseAlibabaPrice. The engine surfaces
// it as a skipped resource with a note rather than a fabricated free price.
func parseHuaweiPrice(raw []byte, expectedCurrency string) (huaweiPriceInfo, error) {
	var resp struct {
		Amount                *float64 `json:"amount"`
		OfficialWebsiteAmount *float64 `json:"official_website_amount"`
		Currency              *string  `json:"currency"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return huaweiPriceInfo{}, err
	}
	amt := 0.0
	if resp.Amount != nil {
		amt = *resp.Amount
	}
	if amt <= 0 && resp.OfficialWebsiteAmount != nil {
		amt = *resp.OfficialWebsiteAmount
	}
	if amt <= 0 {
		return huaweiPriceInfo{}, fmt.Errorf("huawei: rating response carried no positive amount (empty body or a business-level failure); refusing to report a fabricated zero")
	}
	cur := "CNY"
	if resp.Currency != nil && *resp.Currency != "" {
		cur = *resp.Currency
	} else if expectedCurrency != "" {
		cur = expectedCurrency
	}
	return huaweiPriceInfo{Amount: amt, Currency: cur}, nil
}

// --- request builders (shared by multi-cloud mappers) ---

// huaweiProductInfo builds a single DemandProductInfo payload map for the
// Huawei Cloud BSS ListOnDemandResourceRatings API. It centralizes the stable
// scalar fields so individual mappers only pass semantic values. This removes
// the hand-written string keys that previously produced the
// usage_factor="1"/"size" and project_id=region bugs (code review #1/#2).
//
// usage_factor defaults to "Duration" and usage_measure_id to 4 (hour) — the
// documented pair for "inquire the hourly price" (see Huawei BSS docs:
// usageValue=1, usageMeasureID=4). The EIP-by-traffic case overrides these via
// huaweiProductInfoEx (usage_factor="upflow", usage_measure_id=10 for GB).
//
// For linear products billed per-unit (e.g. EVS disks per-GB, bandwidth per-Mbps)
// pass resourceSize > 0 together with sizeMeasureID (17 = GB for EVS, 15 = Mbps
// for bandwidth); they are omitted otherwise. The project_id is injected by the
// backend, never by a mapper.
func huaweiProductInfo(cloudServiceType, resourceType, resourceSpec, region string, resourceSize int, sizeMeasureID int32) map[string]interface{} {
	return huaweiProductInfoEx(cloudServiceType, resourceType, resourceSpec, region, resourceSize, sizeMeasureID, "Duration", 4)
}

// huaweiProductInfoEx is the fully-parameterized variant, used when the
// usage_factor / usage_measure_id differ from the default Duration/hour pair
// (e.g. EIP billed by traffic → usage_factor="upflow", usage_measure_id=10).
func huaweiProductInfoEx(cloudServiceType, resourceType, resourceSpec, region string, resourceSize int, sizeMeasureID int32, usageFactor string, usageMeasureID int32) map[string]interface{} {
	pi := map[string]interface{}{
		"id":                 "1",
		"cloud_service_type": cloudServiceType,
		"resource_type":      resourceType,
		"resource_spec":      resourceSpec,
		"region":             region,
		"usage_factor":       usageFactor,
		"usage_value":        1,
		"usage_measure_id":   usageMeasureID,
		"subscription_num":   1,
	}
	if resourceSize > 0 {
		pi["resource_size"] = resourceSize
	}
	if sizeMeasureID > 0 {
		pi["size_measure_id"] = sizeMeasureID
	}
	return pi
}

// alibabaModule builds a single BSS GetPayAsYouGoPrice ModuleList entry,
// centralizing the three string keys so mappers cannot mistype them (code
// review #8).
func alibabaModule(code, priceType, config string) map[string]string {
	return map[string]string{"ModuleCode": code, "PriceType": priceType, "Config": config}
}
