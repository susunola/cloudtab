package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// TestDirectConnectGatewayExtract asserts the mapper targets the vpc
// InquirePriceCreateDirectConnectGateway action and passes the plan's region
// through unchanged (the API takes no business parameters).
func TestDirectConnectGatewayExtract(t *testing.T) {
	req, err := DirectConnectGateway{}.Extract(parser.PlannedResource{
		Type:   "tencentcloud_dc_gateway",
		Region: "ap-guangzhou",
		After:  map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Product != "vpc" {
		t.Errorf("Product = %q, want vpc", req.Product)
	}
	if req.Action != "InquirePriceCreateDirectConnectGateway" {
		t.Errorf("Action = %q, want InquirePriceCreateDirectConnectGateway", req.Action)
	}
	if req.Region != "ap-guangzhou" {
		t.Errorf("Region = %q, want ap-guangzhou", req.Region)
	}
}

// TestDirectConnectGatewayParse covers the response-shape matrix: the real SDK
// nests the payload under "Response", test mocks use the top level, and the
// discounted RealTotalCost must win over the undiscounted TotalCost. The result
// is always a single PREPAID monthly CNY component.
func TestDirectConnectGatewayParse(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want float64
	}{
		{"nested real cost", `{"Response":{"TotalCost":100,"RealTotalCost":80}}`, 80},
		{"top-level real cost", `{"TotalCost":100,"RealTotalCost":80}`, 80},
		{"nested falls back to TotalCost when no discount", `{"Response":{"TotalCost":100,"RealTotalCost":0}}`, 100},
		{"top-level falls back to TotalCost", `{"TotalCost":100,"RealTotalCost":0}`, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comps, err := DirectConnectGateway{}.Parse(pricing.PriceRequest{}, []byte(tc.raw))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(comps) != 1 {
				t.Fatalf("components = %d, want 1", len(comps))
			}
			c := comps[0]
			if c.MonthlyCost != tc.want {
				t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, tc.want)
			}
			if c.Unit != "MONTH" {
				t.Errorf("Unit = %q, want MONTH (flat prepaid monthly fee)", c.Unit)
			}
			if c.HourlyCost != 0 {
				t.Errorf("HourlyCost = %v, want 0 (prepaid monthly, not hourly)", c.HourlyCost)
			}
			if c.Currency != "CNY" {
				t.Errorf("Currency = %q, want CNY", c.Currency)
			}
			if c.Name == "" {
				t.Error("Name is empty, want a descriptive component name")
			}
		})
	}
}
