package resources

import "strings"

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
