package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

func almostEqDomain(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestTldOf(t *testing.T) {
	cases := map[string]string{
		"example.com":     "com",
		"foo.bar.com.cn":  "cn",
		"EXAMPLE.COM":     "com",
		"example.com.":    "com",
		"com":             "com",
		"  example.net  ": "net",
	}
	for in, want := range cases {
		if got := tldOf(in); got != want {
			t.Fatalf("tldOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDomainExtract(t *testing.T) {
	req, err := DomainRegistration{}.Extract(parser.PlannedResource{
		Type:  "tencentcloud_domain_registration",
		After: map[string]interface{}{"domain_name": "myapp.com", "period": 2},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Product != "domain" || req.Action != "DescribeDomainPriceList" {
		t.Fatalf("unexpected route: %s/%s", req.Product, req.Action)
	}
	tlds := req.Params["TldList"].([]interface{})
	if len(tlds) != 1 || tlds[0] != "com" {
		t.Fatalf("TldList = %v, want [com]", tlds)
	}
	ops := req.Params["Operation"].([]interface{})
	if ops[0] != "new" {
		t.Fatalf("Operation = %v, want [new]", ops)
	}
}

func TestDomainExtractRequiresName(t *testing.T) {
	if _, err := (DomainRegistration{}).Extract(parser.PlannedResource{
		Type:  "tencentcloud_domain_registration",
		After: map[string]interface{}{},
	}); err == nil {
		t.Fatal("expected error for missing domain_name")
	}
}

func TestDomainParseYearlyToMonthly(t *testing.T) {
	req := pricing.PriceRequest{Params: map[string]interface{}{}}
	// Price/RealPrice are whole-yuan (元). RealPrice=96 -> 96 元/year -> /12 = 8 元/mo.
	// Picks the "new" entry over the "renew" entry, and prefers RealPrice.
	raw := []byte(`{"Response":{"PriceList":[
		{"Tld":"com","Year":1,"Price":120,"RealPrice":110,"Operation":"renew"},
		{"Tld":"com","Year":1,"Price":100,"RealPrice":96,"Operation":"new"}
	]}}`)
	comps, err := DomainRegistration{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if !almostEqDomain(comps[0].MonthlyCost, 8.0) {
		t.Fatalf("monthly = %v, want 8.0 (96/12)", comps[0].MonthlyCost)
	}
	if comps[0].Unit != "MONTH" || comps[0].Currency != "CNY" {
		t.Fatalf("unexpected meta: %+v", comps[0])
	}
}

func TestDomainParseFallbackToListPrice(t *testing.T) {
	req := pricing.PriceRequest{Params: map[string]interface{}{}}
	// RealPrice=0 -> fall back to Price. Top-level (no Response) path.
	raw := []byte(`{"PriceList":[{"Tld":"cn","Year":1,"Price":24,"RealPrice":0,"Operation":"new"}]}`)
	comps, err := DomainRegistration{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	// 24 元/year -> /12 = 2 元/mo.
	if !almostEqDomain(comps[0].MonthlyCost, 2.0) {
		t.Fatalf("monthly = %v, want 2.0", comps[0].MonthlyCost)
	}
}

func TestDomainParseEmptyList(t *testing.T) {
	if _, err := (DomainRegistration{}).Parse(pricing.PriceRequest{Params: map[string]interface{}{}},
		[]byte(`{"Response":{"PriceList":[]}}`)); err == nil {
		t.Fatal("expected error for empty price list")
	}
}
