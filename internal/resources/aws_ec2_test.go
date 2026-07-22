package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

func almostEqAWS(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestAWSInstanceExtract(t *testing.T) {
	r := parser.PlannedResource{
		Address: "aws_instance.web",
		Type:    "aws_instance",
		Region:  "us-west-2",
		After: map[string]interface{}{
			"instance_type": "m5.large",
			"tenancy":       "default",
		},
	}
	req, err := AWSInstance{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Provider != "aws" {
		t.Fatalf("Provider = %q, want aws", req.Provider)
	}
	if req.Product != "AmazonEC2" {
		t.Fatalf("Product = %q, want AmazonEC2", req.Product)
	}
	if req.Region != "us-west-2" {
		t.Fatalf("Region = %q, want us-west-2", req.Region)
	}
	// Verify the key filters were built with the right region→location mapping.
	if got := filterValue(req, "instanceType"); got != "m5.large" {
		t.Fatalf("instanceType filter = %q, want m5.large", got)
	}
	if got := filterValue(req, "location"); got != "US West (Oregon)" {
		t.Fatalf("location filter = %q, want US West (Oregon)", got)
	}
	if got := filterValue(req, "tenancy"); got != "Shared" {
		t.Fatalf("tenancy filter = %q, want Shared", got)
	}
	if got := filterValue(req, "operatingSystem"); got != "Linux" {
		t.Fatalf("operatingSystem filter = %q, want Linux", got)
	}
	if got := filterValue(req, "capacitystatus"); got != "Used" {
		t.Fatalf("capacitystatus filter = %q, want Used", got)
	}
}

func TestAWSInstanceExtractMissingType(t *testing.T) {
	r := parser.PlannedResource{
		Address: "aws_instance.web",
		Type:    "aws_instance",
		Region:  "us-east-1",
		After:   map[string]interface{}{},
	}
	if _, err := (AWSInstance{}).Extract(r); err == nil {
		t.Fatal("expected error for missing instance_type, got nil")
	}
}

func TestAWSInstanceExtractDefaultRegion(t *testing.T) {
	r := parser.PlannedResource{
		Type: "aws_instance",
		After: map[string]interface{}{
			"instance_type": "t3.micro",
		},
	}
	req, err := AWSInstance{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Region != "us-east-1" {
		t.Fatalf("Region = %q, want us-east-1 fallback", req.Region)
	}
	if got := filterValue(req, "location"); got != "US East (N. Virginia)" {
		t.Fatalf("location = %q, want US East (N. Virginia)", got)
	}
}

func TestAWSInstanceParse(t *testing.T) {
	req := awsPriceRequest("AmazonEC2", "us-east-1",
		awsFilter("instanceType", "m5.large"),
	)
	// A minimal, realistic Price List document array with an OnDemand USD rate.
	raw := []byte(`[
		{
			"product": {"attributes": {"instanceType": "m5.large"}},
			"terms": {"OnDemand": {"ABC.JRTCKXETXF": {
				"priceDimensions": {"ABC.JRTCKXETXF.6YS6EN2CT7": {
					"unit": "Hrs",
					"pricePerUnit": {"USD": "0.096000000"}
				}}
			}}}
		}
	]`)
	comps, err := AWSInstance{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Currency != "USD" {
		t.Fatalf("Currency = %q, want USD", c.Currency)
	}
	if !almostEqAWS(c.HourlyCost, 0.096) {
		t.Fatalf("HourlyCost = %v, want 0.096", c.HourlyCost)
	}
	if !almostEqAWS(c.MonthlyCost, 0.096*hoursPerMonth) {
		t.Fatalf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.096*hoursPerMonth)
	}
	if c.Name != "EC2 m5.large (Linux, on-demand)" {
		t.Fatalf("Name = %q", c.Name)
	}
}

func TestAWSInstanceParseSkipsZeroPriceSKU(t *testing.T) {
	req := awsPriceRequest("AmazonEC2", "us-east-1")
	// First SKU has a zero price (e.g. Unused capacity); the real rate is second.
	raw := []byte(`[
		{"terms": {"OnDemand": {"A": {"priceDimensions": {"A.1": {
			"unit": "Hrs", "pricePerUnit": {"USD": "0.0000000000"}
		}}}}}},
		{"terms": {"OnDemand": {"B": {"priceDimensions": {"B.1": {
			"unit": "Hrs", "pricePerUnit": {"USD": "0.192000000"}
		}}}}}}
	]`)
	comps, err := AWSInstance{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.192) {
		t.Fatalf("HourlyCost = %v, want 0.192", comps[0].HourlyCost)
	}
}

func TestAWSInstanceParseEmptyList(t *testing.T) {
	req := awsPriceRequest("AmazonEC2", "us-east-1")
	if _, err := (AWSInstance{}).Parse(req, []byte(`[]`)); err == nil {
		t.Fatal("expected error for empty price list, got nil")
	}
}

func TestAWSTenancy(t *testing.T) {
	cases := map[string]string{
		"default":   "Shared",
		"":          "Shared",
		"dedicated": "Dedicated",
		"host":      "Host",
	}
	for in, want := range cases {
		if got := awsTenancy(in); got != want {
			t.Errorf("awsTenancy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAWSLocationUnknownRegionFallsBack(t *testing.T) {
	if got := awsLocation("xx-nowhere-9"); got != awsRegionToLocation["us-east-1"] {
		t.Fatalf("unknown region location = %q, want us-east-1 fallback", got)
	}
	if got := awsLocation(""); got != awsRegionToLocation["us-east-1"] {
		t.Fatalf("empty region location = %q, want us-east-1 fallback", got)
	}
}

// keep the pricing import referenced in case future tests build requests
var _ = pricing.PriceRequest{}
