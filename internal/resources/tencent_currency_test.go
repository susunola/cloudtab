package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/pricing"
)

// TestTencentCurrencyHelper covers the fallback rule used by every Tencent
// mapper whose pricing API returns no per-item Currency field: honour the
// engine-threaded expected currency (USD on the Tencent International site),
// falling back to CNY when unset.
func TestTencentCurrencyHelper(t *testing.T) {
	cases := []struct {
		expected string
		want     string
	}{
		{"USD", "USD"},
		{"CNY", "CNY"},
		{"", "CNY"},
		{"  ", "CNY"},
	}
	for _, c := range cases {
		if got := tencentCurrency(c.expected); got != c.want {
			t.Errorf("tencentCurrency(%q) = %q, want %q", c.expected, got, c.want)
		}
	}
}

// TestTencentMappersHonorExpectedCurrency guards the C1-for-Tencent fix: a
// Tencent mapper that used to hardcode Currency:"CNY" must now label its
// component with the expected currency the engine threads in for the
// International site (USD). Domestic (expected="") must still yield CNY. This
// exercises a representative mix of the previously-hardcoded mappers across
// their distinct response shapes.
func TestTencentMappersHonorExpectedCurrency(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		req  func(expected string) pricing.PriceRequest
		run  func(req pricing.PriceRequest, raw string) (string, error)
	}{
		{
			name: "SQLServer prepaid",
			raw:  `{"Response":{"Price":30000,"OriginalPrice":30000}}`,
			req: func(expected string) pricing.PriceRequest {
				return pricing.PriceRequest{
					ExpectedCurrency: expected,
					Params: map[string]interface{}{
						"InstanceChargeType": "PREPAID", "Memory": 8, "Storage": 100,
					},
				}
			},
			run: func(req pricing.PriceRequest, raw string) (string, error) {
				comps, err := SQLServerInstance{}.Parse(req, []byte(raw))
				if err != nil {
					return "", err
				}
				return comps[0].Currency, nil
			},
		},
		{
			name: "DCDB postpaid",
			raw:  `{"Response":{"Price":100,"OriginalPrice":100}}`,
			req: func(expected string) pricing.PriceRequest {
				return pricing.PriceRequest{
					ExpectedCurrency: expected,
					Params: map[string]interface{}{
						"Paymode": "postpaid", "ShardCount": 2, "ShardMemory": 4,
					},
				}
			},
			run: func(req pricing.PriceRequest, raw string) (string, error) {
				comps, err := DCDBInstance{}.Parse(req, []byte(raw))
				if err != nil {
					return "", err
				}
				return comps[0].Currency, nil
			},
		},
		{
			name: "MySQL via parseTencentPrice (no API currency)",
			raw:  `{"Response":{"Price":12345}}`,
			req: func(expected string) pricing.PriceRequest {
				return pricing.PriceRequest{
					ExpectedCurrency: expected,
					Params:           map[string]interface{}{"PayType": "PRE_PAID", "Memory": 4000, "Volume": 200},
				}
			},
			run: func(req pricing.PriceRequest, raw string) (string, error) {
				comps, err := MySQLInstance{}.Parse(req, []byte(raw))
				if err != nil {
					return "", err
				}
				return comps[0].Currency, nil
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name+"/intl", func(t *testing.T) {
			got, err := c.run(c.req("USD"), c.raw)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if got != "USD" {
				t.Errorf("intl currency = %q, want USD", got)
			}
		})
		t.Run(c.name+"/domestic", func(t *testing.T) {
			got, err := c.run(c.req(""), c.raw)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if got != "CNY" {
				t.Errorf("domestic currency = %q, want CNY", got)
			}
		})
	}
}

// TestParseTencentPriceExplicitCurrencyWins confirms an explicit API Currency
// always overrides the expected currency (an intl request that gets a CNY quote
// back is labelled CNY, never blindly USD).
func TestParseTencentPriceExplicitCurrencyWins(t *testing.T) {
	p, err := parseTencentPrice([]byte(`{"Response":{"Price":100,"Currency":"CNY"}}`), "USD")
	if err != nil {
		t.Fatalf("parseTencentPrice error: %v", err)
	}
	if p.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY (explicit API currency must win)", p.Currency)
	}
}
