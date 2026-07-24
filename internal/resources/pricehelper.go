package resources

import (
	"encoding/json"
	"strings"
)

// hoursPerMonth is the conventional Tencent Cloud billing month (30.4 days).
// Tencent's own console uses ~730h for POSTPAID monthly estimates.
const hoursPerMonth = 730.0

// monthlyFromPrice converts an InquiryPrice* discounted price into a monthly
// figure, deciding PREPAID vs POSTPAID from the official ChargeUnit field
// rather than guessing from OriginalPrice.
//
//   - ChargeUnit == "HOUR"  → POSTPAID: unitPriceDiscount is 元/小时, ×730 for monthly.
//   - ChargeUnit == "MONTH" → PREPAID:  discountPrice is already the monthly total.
//   - other / empty         → fall back to the POSTPAID hourly assumption; if
//     discountPrice is set and unitPriceDiscount is 0 we treat it as a fixed price.
//
// Returns (monthlyCost, hourlyCost). hourlyCost is 0 for non-hourly billing.
func monthlyFromPrice(chargeUnit string, unitPriceDiscount, discountPrice float64) (monthly, hourly float64) {
	switch strings.ToUpper(strings.TrimSpace(chargeUnit)) {
	case "HOUR":
		return unitPriceDiscount * hoursPerMonth, unitPriceDiscount
	case "MONTH":
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

// discountedYuanFromCents resolves the standard cents-based (分) DescribePrice
// response used by the mariadb / sqlserver / dcdb mappers, which all return
// int64 Price/OriginalPrice both at the top level AND under a nested "Response"
// wrapper. It encodes the three identical decisions those mappers previously
// duplicated verbatim:
//
//  1. Dual-path: prefer the nested Response pair when it carries data (real SDK
//     ToJsonString output), else use the top-level pair (test mocks).
//  2. Discount fallback: prefer the discounted Price; fall back to OriginalPrice
//     when the API returned no discount (Price == 0).
//  3. Unit: divide 分 by 100 to get 元.
//
// Returns the resolved price in 元.
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
// 元-based mappers (lighthouse / ecm / gaap) share this "prefer discount, fall
// back to original" rule; only the surrounding struct shape differs, so each
// caller selects its own (discount, original) pair first and then applies this.
func preferDiscount(discount, original float64) float64 {
	if discount > 0 {
		return discount
	}
	return original
}

// splitByBilling maps a single per-unit 元 price onto cloudtab's (monthly,
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
// both in 分, plus a Currency string.
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

// parseAlibabaPrice unmarshals a raw Alibaba Cloud BSS GetPayAsYouGoPrice
// response into a simplified price info struct. It sums CostAfterDiscount
// across all module details, falling back to OriginalCost. Currency defaults
// to "CNY".
func parseAlibabaPrice(raw []byte) (alibabaPriceInfo, error) {
	var resp struct {
		Data struct {
			Currency      string `json:"Currency"`
			ModuleDetails struct {
				ModuleDetail []struct {
					CostAfterDiscount float64 `json:"CostAfterDiscount"`
					OriginalCost      float64 `json:"OriginalCost"`
				} `json:"ModuleDetail"`
			} `json:"ModuleDetails"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return alibabaPriceInfo{}, err
	}
	cur := resp.Data.Currency
	if cur == "" {
		cur = "CNY"
	}
	var total float64
	for _, md := range resp.Data.ModuleDetails.ModuleDetail {
		v := md.CostAfterDiscount
		if v <= 0 {
			v = md.OriginalCost
		}
		total += v
	}
	return alibabaPriceInfo{PriceYuan: total, Currency: cur}, nil
}

// --- Huawei Cloud helpers ---

// huaweiPriceInfo is the common BSS ListOnDemandResourceRatings response shape.
type huaweiPriceInfo struct {
	Amount   float64 // amount in 元 (or USD for international)
	Currency string
}

// parseHuaweiPrice unmarshals a raw Huawei Cloud BSS ListOnDemandResourceRatings
// response. It reads the top-level "amount" field, falling back to
// "official_website_amount". Currency defaults to "CNY".
func parseHuaweiPrice(raw []byte) (huaweiPriceInfo, error) {
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
	cur := "CNY"
	if resp.Currency != nil && *resp.Currency != "" {
		cur = *resp.Currency
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
