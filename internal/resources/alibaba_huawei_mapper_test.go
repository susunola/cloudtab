package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/pricing"
)

// mapperParse is the common Parse signature shared by the Alibaba/Huawei
// mappers under test.
type mapperParse func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error)

// TestAlibabaHuaweiMapperParse exercises the per-mapper Parse methods for the
// common Alibaba/Huawei mappers that wrap a single hourly rate into one
// CostComponent via simpleHourlyCost. It asserts exactly one component, a
// positive MonthlyCost equal to hourly x hoursPerMonth, the CNY currency from
// the fixture, and a non-empty Name.
func TestAlibabaHuaweiMapperParse(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		parse        mapperParse
		wantName     string
		wantHourly   float64
		wantCurrency string
	}{
		{
			name: "AlibabaECS",
			raw:  `{"Data":{"Currency":"CNY","ModuleDetails":{"ModuleDetail":[{"ModuleCode":"InstanceType","CostAfterDiscount":0.1}]}}}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return AlibabaECS{}.Parse(req, raw)
			},
			wantName:     "Alibaba ECS",
			wantHourly:   0.1,
			wantCurrency: "CNY",
		},
		{
			name: "AlibabaRDS",
			raw:  `{"Data":{"Currency":"CNY","ModuleDetails":{"ModuleDetail":[{"ModuleCode":"DBInstanceClass","CostAfterDiscount":0.2}]}}}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return AlibabaRDS{}.Parse(req, raw)
			},
			wantName:     "Alibaba RDS",
			wantHourly:   0.2,
			wantCurrency: "CNY",
		},
		{
			name: "AlibabaDisk",
			raw:  `{"Data":{"Currency":"CNY","ModuleDetails":{"ModuleDetail":[{"ModuleCode":"DataDisk","CostAfterDiscount":0.05}]}}}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return AlibabaDisk{}.Parse(req, raw)
			},
			wantName:     "Alibaba Disk",
			wantHourly:   0.05,
			wantCurrency: "CNY",
		},
		{
			name: "HuaweiECS",
			raw:  `{"amount":0.3,"currency":"CNY"}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return HuaweiECS{}.Parse(req, raw)
			},
			wantName:     "Huawei ECS",
			wantHourly:   0.3,
			wantCurrency: "CNY",
		},
		{
			name: "HuaweiEVS",
			raw:  `{"amount":0.07,"currency":"CNY"}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return HuaweiEVS{}.Parse(req, raw)
			},
			wantName:     "Huawei EVS",
			wantHourly:   0.07,
			wantCurrency: "CNY",
		},
		{
			name: "HuaweiRDS",
			raw:  `{"amount":0.15,"currency":"CNY"}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return HuaweiRDS{}.Parse(req, raw)
			},
			wantName:     "Huawei RDS",
			wantHourly:   0.15,
			wantCurrency: "CNY",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := pricing.PriceRequest{ExpectedCurrency: "", Params: map[string]interface{}{}}
			comps, err := c.parse(req, []byte(c.raw))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if len(comps) != 1 {
				t.Fatalf("got %d components, want exactly 1", len(comps))
			}
			got := comps[0]
			if got.Name == "" {
				t.Errorf("Name is empty, want %q", c.wantName)
			}
			if got.Name != c.wantName {
				t.Errorf("Name = %q, want %q", got.Name, c.wantName)
			}
			wantMonthly := c.wantHourly * hoursPerMonth
			if !almostEqual(got.MonthlyCost, wantMonthly) {
				t.Errorf("MonthlyCost = %v, want %v", got.MonthlyCost, wantMonthly)
			}
			if got.MonthlyCost <= 0 {
				t.Errorf("MonthlyCost = %v, want positive", got.MonthlyCost)
			}
			if got.HourlyCost != c.wantHourly {
				t.Errorf("HourlyCost = %v, want %v", got.HourlyCost, c.wantHourly)
			}
			if got.Unit != "HOUR" {
				t.Errorf("Unit = %q, want HOUR", got.Unit)
			}
			if got.Currency != c.wantCurrency {
				t.Errorf("Currency = %q, want %q", got.Currency, c.wantCurrency)
			}
		})
	}
}

// TestMapperExpectedCurrencyUSD guards the C1 fix at the mapper level: when an
// International Alibaba/Huawei response omits its currency field, the expected
// currency (USD) threaded by the engine must propagate to the component, not be
// silently assumed CNY.
func TestMapperExpectedCurrencyUSD(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		parse mapperParse
	}{
		{
			name: "AlibabaECS omits currency -> USD",
			raw:  `{"Data":{"ModuleDetails":{"ModuleDetail":[{"ModuleCode":"InstanceType","CostAfterDiscount":0.1}]}}}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return AlibabaECS{}.Parse(req, raw)
			},
		},
		{
			name: "HuaweiECS omits currency -> USD",
			raw:  `{"amount":0.3}`,
			parse: func(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
				return HuaweiECS{}.Parse(req, raw)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := pricing.PriceRequest{ExpectedCurrency: "USD", Params: map[string]interface{}{}}
			comps, err := c.parse(req, []byte(c.raw))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if len(comps) != 1 {
				t.Fatalf("got %d components, want exactly 1", len(comps))
			}
			if comps[0].Currency != "USD" {
				t.Errorf("Currency = %q, want USD (expected currency must propagate when fixture omits it)", comps[0].Currency)
			}
		})
	}
}
