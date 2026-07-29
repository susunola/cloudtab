package resources

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/pricing"
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
			want: 50.0, // 5000cents = 50CNY, discount preferred
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

// TestParseAlibabaHuaweiCurrency guards C1: an International Alibaba/Huawei
// response that omits its currency field must be labelled USD (the engine
// threads the expected currency for the provider's site), NOT silently assumed
// CNY — otherwise an intl USD quote would be summed with CNY totals. An
// explicit API currency always wins over the expected one.
func TestParseAlibabaHuaweiCurrency(t *testing.T) {
	alibabaEmpty := `{"Data":{"Currency":"","ModuleDetails":{"ModuleDetail":[{"ModuleCode":"X","CostAfterDiscount":10}]}}}`
	alibabaCNY := `{"Data":{"Currency":"CNY","ModuleDetails":{"ModuleDetail":[{"ModuleCode":"X","CostAfterDiscount":10}]}}}`
	huaweiEmpty := `{"amount":5,"currency":""}`
	huaweiUSD := `{"amount":5,"currency":"USD"}`

	cases := []struct {
		name     string
		raw      string
		parse    func(raw, expected string) (string, error)
		expected string
		want     string
	}{
		{
			name: "alibaba intl: empty currency -> USD",
			raw:  alibabaEmpty,
			parse: func(raw, expected string) (string, error) {
				p, err := parseAlibabaPrice([]byte(raw), expected)
				return p.Currency, err
			},
			expected: "USD",
			want:     "USD",
		},
		{
			name: "alibaba domestic: empty currency -> CNY",
			raw:  alibabaEmpty,
			parse: func(raw, expected string) (string, error) {
				p, err := parseAlibabaPrice([]byte(raw), expected)
				return p.Currency, err
			},
			expected: "CNY",
			want:     "CNY",
		},
		{
			name: "alibaba: explicit currency wins over expected",
			raw:  alibabaCNY,
			parse: func(raw, expected string) (string, error) {
				p, err := parseAlibabaPrice([]byte(raw), expected)
				return p.Currency, err
			},
			expected: "USD",
			want:     "CNY",
		},
		{
			name: "huawei intl: empty currency -> USD",
			raw:  huaweiEmpty,
			parse: func(raw, expected string) (string, error) {
				p, err := parseHuaweiPrice([]byte(raw), expected)
				return p.Currency, err
			},
			expected: "USD",
			want:     "USD",
		},
		{
			name: "huawei domestic: empty currency -> CNY",
			raw:  huaweiEmpty,
			parse: func(raw, expected string) (string, error) {
				p, err := parseHuaweiPrice([]byte(raw), expected)
				return p.Currency, err
			},
			expected: "CNY",
			want:     "CNY",
		},
		{
			name: "huawei: explicit currency wins over expected",
			raw:  huaweiUSD,
			parse: func(raw, expected string) (string, error) {
				p, err := parseHuaweiPrice([]byte(raw), expected)
				return p.Currency, err
			},
			expected: "CNY",
			want:     "USD",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.parse(c.raw, c.expected)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got != c.want {
				t.Errorf("currency = %q, want %q", got, c.want)
			}
		})
	}
}

func TestValidateTencentPaidPrice(t *testing.T) {
	positive := []output.CostComponent{{MonthlyCost: 1}}
	cases := []struct {
		name    string
		raw     string
		comps   []output.CostComponent
		wantErr bool
	}{
		{name: "top-level business error", raw: `{"Error":{"Code":"InternalError","Message":"quote failed"}}`, comps: positive, wantErr: true},
		{name: "nested business error", raw: `{"Response":{"Error":{"Code":"InternalError","Message":"quote failed"}}}`, comps: positive, wantErr: true},
		{name: "no components", raw: `{}`, wantErr: true},
		{name: "zero-only components", raw: `{}`, comps: []output.CostComponent{{MonthlyCost: 0, HourlyCost: 0}}, wantErr: true},
		{name: "non-finite components", raw: `{}`, comps: []output.CostComponent{{MonthlyCost: math.NaN(), HourlyCost: math.Inf(1)}}, wantErr: true},
		{name: "optional zero with positive monthly component", raw: `{}`, comps: []output.CostComponent{{MonthlyCost: 0}, {MonthlyCost: 2.5}}},
		{name: "optional zero with positive hourly component", raw: `{}`, comps: []output.CostComponent{{MonthlyCost: 0}, {HourlyCost: 0.01}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTencentPaidPrice([]byte(tc.raw), tc.comps)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateTencentPaidPrice() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestPaidTencentMappersRejectInvalidPriceResponses(t *testing.T) {
	reg := DefaultRegistry()
	types := reg.Keys()
	sort.Strings(types)

	seen := make(map[reflect.Type]struct{})
	paid := make([]struct {
		name   string
		mapper Mapper
	}, 0, 20)
	for _, tfType := range types {
		if !strings.HasPrefix(tfType, "tencentcloud_") {
			continue
		}
		mapper, ok := reg.Lookup(tfType)
		if !ok {
			t.Fatalf("registered mapper %s cannot be looked up", tfType)
		}
		if _, static := mapper.(StaticMapper); static {
			continue
		}
		if _, usage := mapper.(UsageMapper); usage {
			continue
		}
		mapperType := reflect.TypeOf(mapper)
		if _, duplicate := seen[mapperType]; duplicate {
			continue
		}
		seen[mapperType] = struct{}{}
		paid = append(paid, struct {
			name   string
			mapper Mapper
		}{name: tfType, mapper: mapper})
	}
	if len(paid) != 20 {
		t.Fatalf("found %d unique paid Tencent mappers, want 20", len(paid))
	}

	invalid := []struct {
		name string
		raw  string
	}{
		{name: "empty object", raw: `{}`},
		{name: "empty Response", raw: `{"Response":{}}`},
		{name: "business error", raw: `{"Response":{"Error":{"Code":"InternalError","Message":"quote failed"}}}`},
	}
	for _, entry := range paid {
		for _, tc := range invalid {
			t.Run(entry.name+"/"+tc.name, func(t *testing.T) {
				comps, err := entry.mapper.Parse(pricing.PriceRequest{Params: map[string]interface{}{}}, []byte(tc.raw))
				if err == nil {
					t.Fatalf("Parse returned %d components without error for invalid paid response %s", len(comps), tc.raw)
				}
			})
		}
	}
}

// TestExpectedCurrencyFor mirrors the engine method that decides the expected
// currency per provider/site. Intl Tencent/Alibaba/Huawei -> USD; every
// Chinese-mainland site (and AWS) -> CNY.
// (Defined in the pricing package's engine_test.go since it exercises Engine.)
