package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// monthsPerYear folds a per-year domain registration price into a monthly
// run-rate so it lines up with every other product's monthly reporting.
const monthsPerYear = 12.0

// DomainRegistration handles `tencentcloud_domain_registration`.
//
// Pricing API (domain): DescribeDomainPriceList (Describe, not Inquiry).
// Docs: https://cloud.tencent.com/document/api/242/48941
//
// Domain pricing is fundamentally different from instance products: the API
// returns a PriceList of {Tld, Year, Operation, Price, RealPrice} tuples. There
// is no region, no charge type, and prices are one-off yearly fees, not monthly
// instance run-rates. We query the "new" (first-purchase) price for 1 year for
// the resource's TLD, then divide by 12 to express a comparable monthly figure.
//
// IMPORTANT — pricing unit: the SDK types Price/RealPrice as uint64 but the API
// docs and struct comments (域名原价/域名现价) do NOT state a unit. Empirically
// this endpoint returns whole-yuan (元) integers (e.g. 55 for a ¥55/year .com),
// NOT cents. We therefore treat the value as 元 directly. If a future SDK/API
// revision switches to 分, change domainPriceUnitDivisor to 100.0.
type DomainRegistration struct{}

// domainPriceUnitDivisor converts the raw integer Price/RealPrice to 元.
// The domain API reports whole-yuan integers, so the divisor is 1.
const domainPriceUnitDivisor = 1.0

// tldOf extracts the TLD (suffix) from a domain name, e.g. "example.com" -> "com".
// It accepts values with or without a leading dot and is case-insensitive.
func tldOf(domain string) string {
	d := strings.TrimSpace(strings.ToLower(domain))
	d = strings.TrimSuffix(d, ".")
	if i := strings.LastIndex(d, "."); i >= 0 && i < len(d)-1 {
		return d[i+1:]
	}
	return d
}

func (DomainRegistration) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	domain := strings.TrimSpace(getStr(r.After, "domain_name"))
	if domain == "" {
		return pricing.PriceRequest{}, fmt.Errorf("tencentcloud_domain_registration requires domain_name")
	}
	tld := tldOf(domain)
	if tld == "" {
		return pricing.PriceRequest{}, fmt.Errorf("cannot derive TLD from domain_name %q", domain)
	}

	// period is the registration length in years; used only to pick a matching
	// PriceList entry. We still report a monthly run-rate.
	year := getInt(r.After, "period")
	if year <= 0 {
		year = 1
	}

	params := map[string]interface{}{
		"TldList":   []interface{}{tld},
		"Year":      []interface{}{year},
		"Operation": []interface{}{"new"},
	}

	return pricing.PriceRequest{
		Product: "domain",
		Action:  "DescribeDomainPriceList",
		Region:  r.Region,
		Params:  params,
	}, nil
}

func (DomainRegistration) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	type priceInfo struct {
		Tld       string `json:"Tld"`
		Year      uint64 `json:"Year"`
		Price     uint64 `json:"Price"`     // list price, 分
		RealPrice uint64 `json:"RealPrice"` // current price, 分
		Operation string `json:"Operation"`
	}
	type priceBlock struct {
		PriceList []priceInfo `json:"PriceList"`
	}
	var wrap struct {
		priceBlock
		Response struct {
			priceBlock
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}

	list := wrap.PriceList
	if len(wrap.Response.PriceList) > 0 {
		list = wrap.Response.PriceList
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("domain price list is empty")
	}

	// Pick the "new" entry (case-insensitive); fall back to the first entry.
	chosen := list[0]
	for _, e := range list {
		if strings.EqualFold(e.Operation, "new") {
			chosen = e
			break
		}
	}

	// Prefer the discounted RealPrice; fall back to the list Price. The raw
	// value is a per-year fee in 元 (see domainPriceUnitDivisor): divide by 12
	// for a comparable monthly run-rate.
	priceInt := chosen.RealPrice
	if priceInt == 0 {
		priceInt = chosen.Price
	}
	yearlyYuan := float64(priceInt) / domainPriceUnitDivisor
	monthly := yearlyYuan / monthsPerYear

	return []output.CostComponent{{
		Name:        fmt.Sprintf("Domain .%s registration (yearly %.2f CNY)", chosen.Tld, yearlyYuan),
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: monthly,
		Currency:    "CNY",
	}}, nil
}
