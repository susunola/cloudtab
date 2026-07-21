package resources

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestMonthlyFromPrice(t *testing.T) {
	cases := []struct {
		name              string
		chargeUnit        string
		unitPriceDiscount float64
		discountPrice     float64
		wantMonthly       float64
		wantHourly        float64
	}{
		{
			name:              "hourly postpaid multiplies by 730",
			chargeUnit:        "HOUR",
			unitPriceDiscount: 0.5,
			discountPrice:     0,
			wantMonthly:       0.5 * hoursPerMonth,
			wantHourly:        0.5,
		},
		{
			name:          "monthly prepaid uses discount price directly",
			chargeUnit:    "MONTH",
			discountPrice: 120.0,
			wantMonthly:   120.0,
			wantHourly:    0,
		},
		{
			name:              "day rate scales by ~30.4 days",
			chargeUnit:        "DAY",
			unitPriceDiscount: 2.0,
			wantMonthly:       2.0 * (hoursPerMonth / 24),
			wantHourly:        0,
		},
		{
			name:              "unknown unit with hourly price assumed hourly",
			chargeUnit:        "",
			unitPriceDiscount: 1.0,
			wantMonthly:       1.0 * hoursPerMonth,
			wantHourly:        1.0,
		},
		{
			name:          "unknown unit with fixed price only",
			chargeUnit:    "weird",
			discountPrice: 42.0,
			wantMonthly:   42.0,
			wantHourly:    0,
		},
		{
			name:              "case and whitespace insensitive",
			chargeUnit:        "  hour ",
			unitPriceDiscount: 0.1,
			wantMonthly:       0.1 * hoursPerMonth,
			wantHourly:        0.1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMonthly, gotHourly := monthlyFromPrice(c.chargeUnit, c.unitPriceDiscount, c.discountPrice)
			if !almostEqual(gotMonthly, c.wantMonthly) {
				t.Errorf("monthly = %v, want %v", gotMonthly, c.wantMonthly)
			}
			if !almostEqual(gotHourly, c.wantHourly) {
				t.Errorf("hourly = %v, want %v", gotHourly, c.wantHourly)
			}
		})
	}
}
