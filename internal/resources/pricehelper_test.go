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

func TestDiscountedYuanFromCents(t *testing.T) {
	cases := []struct {
		name                                   string
		topPrice, topOrig, respPrice, respOrig int64
		want                                   float64
	}{
		{
			name:     "top-level discounted price (test-mock shape)",
			topPrice: 5000, topOrig: 8000,
			want: 50.0, // 5000分 = 50元, discount preferred
		},
		{
			name:     "top-level falls back to original when price is 0",
			topPrice: 0, topOrig: 8000,
			want: 80.0,
		},
		{
			name:     "nested Response wins when populated (real SDK shape)",
			topPrice: 0, topOrig: 0,
			respPrice: 12345, respOrig: 20000,
			want: 123.45,
		},
		{
			name:     "nested Response preferred over top-level even if top has data",
			topPrice: 999, topOrig: 999,
			respPrice: 5000, respOrig: 8000,
			want: 50.0,
		},
		{
			name:      "nested Response with zero price falls back to nested original",
			respPrice: 0, respOrig: 20000,
			want: 200.0,
		},
		{
			name: "all zero yields 0",
			want: 0.0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := discountedYuanFromCents(c.topPrice, c.topOrig, c.respPrice, c.respOrig)
			if !almostEqual(got, c.want) {
				t.Errorf("discountedYuanFromCents(%d,%d,%d,%d) = %v, want %v",
					c.topPrice, c.topOrig, c.respPrice, c.respOrig, got, c.want)
			}
		})
	}
}

func TestSplitByBilling(t *testing.T) {
	t.Run("prepaid: price is the monthly total, hourly 0", func(t *testing.T) {
		monthly, hourly := splitByBilling(120.0, false)
		if !almostEqual(monthly, 120.0) || !almostEqual(hourly, 0) {
			t.Errorf("prepaid = (%v,%v), want (120,0)", monthly, hourly)
		}
	})
	t.Run("postpaid: price is hourly, monthly = hourly*730", func(t *testing.T) {
		monthly, hourly := splitByBilling(0.5, true)
		if !almostEqual(hourly, 0.5) || !almostEqual(monthly, 0.5*hoursPerMonth) {
			t.Errorf("postpaid = (%v,%v), want (%v,0.5)", monthly, hourly, 0.5*hoursPerMonth)
		}
	})
}

func TestPreferDiscount(t *testing.T) {
	if got := preferDiscount(50.0, 80.0); !almostEqual(got, 50.0) {
		t.Errorf("discount present: got %v, want 50", got)
	}
	if got := preferDiscount(0, 80.0); !almostEqual(got, 80.0) {
		t.Errorf("no discount: got %v, want 80 (fall back to original)", got)
	}
	if got := preferDiscount(0, 0); !almostEqual(got, 0) {
		t.Errorf("both zero: got %v, want 0", got)
	}
}
