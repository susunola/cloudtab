package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

func TestAWSLBExtractDefaultsToApplication(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "aws_lb",
		Region: "us-east-1",
		After:  map[string]interface{}{},
	}
	req, err := AWSLB{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Provider != "aws" || req.Product != "AWSELB" {
		t.Fatalf("route = %s/%s, want aws/AWSELB", req.Provider, req.Product)
	}
	if got := filterValue(req, "productFamily"); got != "Load Balancer-Application" {
		t.Fatalf("productFamily = %q, want Load Balancer-Application", got)
	}
	if got := filterValue(req, "locationType"); got != "AWS Region" {
		t.Fatalf("locationType = %q, want AWS Region", got)
	}
}

func TestAWSLBExtractNetworkAndGateway(t *testing.T) {
	for tfType, wantFamily := range map[string]string{
		"network": "Load Balancer-Network",
		"gateway": "Load Balancer-Gateway",
	} {
		r := parser.PlannedResource{
			Type:   "aws_lb",
			Region: "us-east-1",
			After:  map[string]interface{}{"load_balancer_type": tfType},
		}
		req, err := AWSLB{}.Extract(r)
		if err != nil {
			t.Fatalf("Extract(%s) error = %v", tfType, err)
		}
		if got := filterValue(req, "productFamily"); got != wantFamily {
			t.Errorf("type %s: productFamily = %q, want %q", tfType, got, wantFamily)
		}
	}
}

func TestAWSLBExtractUnsupportedType(t *testing.T) {
	if _, err := (AWSLB{}).Extract(parser.PlannedResource{
		Type:  "aws_lb",
		After: map[string]interface{}{"load_balancer_type": "quantum"},
	}); err == nil {
		t.Fatal("expected error for unsupported load_balancer_type")
	}
}

func TestAWSELBClassicExtract(t *testing.T) {
	req, err := AWSELB{}.Extract(parser.PlannedResource{
		Type:   "aws_elb",
		Region: "eu-west-1",
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got := filterValue(req, "productFamily"); got != "Load Balancer" {
		t.Fatalf("productFamily = %q, want Load Balancer", got)
	}
	if got := filterValue(req, "location"); got != "EU (Ireland)" {
		t.Fatalf("location = %q, want EU (Ireland)", got)
	}
}

func TestParseELBPicksLoadBalancerUsage(t *testing.T) {
	req := awsELBRequest("us-east-1", "Load Balancer-Application")
	// Multiple SKUs: the LCU one comes first and must be skipped; the fixed
	// hourly LoadBalancerUsage SKU is the one we want.
	raw := []byte(`[
		{
			"product": {"attributes": {"usagetype": "USE1-LCUUsage"}},
			"terms": {"OnDemand": {"A": {"priceDimensions": {"A.1": {
				"unit": "LCU-Hrs", "pricePerUnit": {"USD": "0.008000000"}
			}}}}}
		},
		{
			"product": {"attributes": {"usagetype": "USE1-LoadBalancerUsage"}},
			"terms": {"OnDemand": {"B": {"priceDimensions": {"B.1": {
				"unit": "Hrs", "pricePerUnit": {"USD": "0.022500000"}
			}}}}}
		}
	]`)
	comps, err := AWSLB{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	c := comps[0]
	if !almostEqAWS(c.HourlyCost, 0.0225) {
		t.Fatalf("HourlyCost = %v, want 0.0225 (LoadBalancerUsage, not LCU)", c.HourlyCost)
	}
	if !almostEqAWS(c.MonthlyCost, 0.0225*hoursPerMonth) {
		t.Fatalf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.0225*hoursPerMonth)
	}
	if c.Name != "ALB hourly (base, excl. LCU/data)" {
		t.Fatalf("Name = %q", c.Name)
	}
}

func TestParseELBNoLoadBalancerUsage(t *testing.T) {
	req := awsELBRequest("us-east-1", "Load Balancer-Application")
	raw := []byte(`[
		{"product": {"attributes": {"usagetype": "USE1-LCUUsage"}},
		 "terms": {"OnDemand": {"A": {"priceDimensions": {"A.1": {
			"unit": "LCU-Hrs", "pricePerUnit": {"USD": "0.008"}
		}}}}}}
	]`)
	if _, err := (AWSLB{}).Parse(req, raw); err == nil {
		t.Fatal("expected error when no LoadBalancerUsage SKU present")
	}
}

func TestAWSLBFamilyLabels(t *testing.T) {
	cases := map[string]string{
		"Load Balancer-Application": "ALB",
		"Load Balancer-Network":     "NLB",
		"Load Balancer-Gateway":     "GWLB",
		"Load Balancer":             "Classic LB",
	}
	for family, want := range cases {
		if got := awsLBLabel(family); got != want {
			t.Errorf("awsLBLabel(%q) = %q, want %q", family, got, want)
		}
	}
}
