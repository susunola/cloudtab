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
	if wrap.Response.Currency != "" {
		p.Currency = wrap.Response.Currency
	}
	if p.Currency == "" {
		p.Currency = "CNY"
	}
	return p, nil
}
