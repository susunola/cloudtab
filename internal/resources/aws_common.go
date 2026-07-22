package resources

// Shared helpers for the AWS Price List mappers.
//
// The AWS Price List API returns, per matching product, a self-contained JSON
// document. The two parts we care about are:
//
//	{
//	  "product":   { "attributes": { "instanceType": "m5.large", ... } },
//	  "terms": {
//	    "OnDemand": {
//	      "<offerTermCode>": {
//	        "priceDimensions": {
//	          "<rateCode>": {
//	            "unit": "Hrs",
//	            "pricePerUnit": { "USD": "0.096000000" },
//	            "description": "..."
//	          }
//	        }
//	      }
//	    }
//	  }
//	}
//
// The backend (internal/pricing/aws_backend.go) returns a JSON ARRAY of these
// documents. These helpers parse that array and pull the first usable OnDemand
// USD unit price, which mappers convert into monthly/hourly CostComponents.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/susunola/cloudtab/internal/pricing"
)

// awsCurrency is the currency the Price List API quotes in.
const awsCurrency = "USD"

// awsDefaultRegion is used when a plan does not carry an explicit AWS region
// (AWS resources usually inherit the provider block's region, which the parser
// resolves; this is the last-resort fallback so the Location filter is valid).
const awsDefaultRegion = "us-east-1"

// awsPriceDimension is one rate line inside an OnDemand term.
type awsPriceDimension struct {
	Unit         string            `json:"unit"`
	PricePerUnit map[string]string `json:"pricePerUnit"`
	Description  string            `json:"description"`
}

// awsProductDoc is the subset of an AWS Price List product document we decode.
type awsProductDoc struct {
	Product struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]awsPriceDimension `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// awsUnitPrice is the resolved on-demand price for one product SKU.
type awsUnitPrice struct {
	USD  float64
	Unit string // e.g. "Hrs", "GB-Mo"
}

// parseAWSPriceList decodes the backend's JSON array of product documents and
// returns the first document that carries a non-zero OnDemand USD price.
//
// AWS often returns multiple SKUs for one filter set (e.g. Used vs Unused
// capacity reservations). We pick the first with a positive price, which for a
// tightly-filtered request is the on-demand rate we want. When several are
// plausible the mapper should tighten its filters rather than rely on ordering;
// picking the first positive price is a safe default that never returns 0.
func parseAWSPriceList(raw []byte) (awsUnitPrice, error) {
	var docs []json.RawMessage
	if err := json.Unmarshal(raw, &docs); err != nil {
		return awsUnitPrice{}, fmt.Errorf("aws price list: not a JSON array: %w", err)
	}
	if len(docs) == 0 {
		return awsUnitPrice{}, fmt.Errorf("aws price list: no matching products (check filters/region)")
	}
	var firstErr error
	for _, d := range docs {
		var doc awsProductDoc
		if err := json.Unmarshal(d, &doc); err != nil {
			firstErr = err
			continue
		}
		if p, ok := firstOnDemandUSD(doc); ok {
			return p, nil
		}
	}
	if firstErr != nil {
		return awsUnitPrice{}, fmt.Errorf("aws price list: could not decode any product: %w", firstErr)
	}
	return awsUnitPrice{}, fmt.Errorf("aws price list: no positive OnDemand USD price found")
}

// firstOnDemandUSD walks the OnDemand terms and returns the first positive USD
// price dimension.
func firstOnDemandUSD(doc awsProductDoc) (awsUnitPrice, bool) {
	for _, term := range doc.Terms.OnDemand {
		for _, dim := range term.PriceDimensions {
			usdStr, ok := dim.PricePerUnit[awsCurrency]
			if !ok {
				continue
			}
			usd, err := strconv.ParseFloat(strings.TrimSpace(usdStr), 64)
			if err != nil || usd <= 0 {
				continue
			}
			return awsUnitPrice{USD: usd, Unit: dim.Unit}, true
		}
	}
	return awsUnitPrice{}, false
}

// parseAWSPriceListMatching is like parseAWSPriceList but only considers
// products whose "usagetype" attribute contains the given substring. Several
// AWS services return multiple SKUs for one filter set that differ only by
// usagetype (e.g. ELB returns LoadBalancerUsage, LCUUsage and DataProcessing);
// this lets a mapper pin the one fixed line it wants. Falls back with a clear
// error when no product matches.
func parseAWSPriceListMatching(raw []byte, usageTypeContains string) (awsUnitPrice, error) {
	var docs []json.RawMessage
	if err := json.Unmarshal(raw, &docs); err != nil {
		return awsUnitPrice{}, fmt.Errorf("aws price list: not a JSON array: %w", err)
	}
	if len(docs) == 0 {
		return awsUnitPrice{}, fmt.Errorf("aws price list: no matching products (check filters/region)")
	}
	for _, d := range docs {
		var doc awsProductDoc
		if err := json.Unmarshal(d, &doc); err != nil {
			continue
		}
		ut := doc.Product.Attributes["usagetype"]
		if usageTypeContains != "" && !strings.Contains(ut, usageTypeContains) {
			continue
		}
		if p, ok := firstOnDemandUSD(doc); ok {
			return p, nil
		}
	}
	return awsUnitPrice{}, fmt.Errorf("aws price list: no positive OnDemand USD price with usagetype containing %q", usageTypeContains)
}

// awsRegionToLocation maps an AWS region code to the human-readable "location"
// value the Price List API filters on. The Price List API does NOT accept
// region codes; it uses the console location name. This covers the commonly
// used commercial regions; unknown regions fall back to us-east-1's location so
// pricing still returns a (clearly-labelled) figure rather than failing.
var awsRegionToLocation = map[string]string{
	"us-east-1":      "US East (N. Virginia)",
	"us-east-2":      "US East (Ohio)",
	"us-west-1":      "US West (N. California)",
	"us-west-2":      "US West (Oregon)",
	"ca-central-1":   "Canada (Central)",
	"eu-west-1":      "EU (Ireland)",
	"eu-west-2":      "EU (London)",
	"eu-west-3":      "EU (Paris)",
	"eu-central-1":   "EU (Frankfurt)",
	"eu-north-1":     "EU (Stockholm)",
	"eu-south-1":     "EU (Milan)",
	"ap-east-1":      "Asia Pacific (Hong Kong)",
	"ap-south-1":     "Asia Pacific (Mumbai)",
	"ap-northeast-1": "Asia Pacific (Tokyo)",
	"ap-northeast-2": "Asia Pacific (Seoul)",
	"ap-northeast-3": "Asia Pacific (Osaka)",
	"ap-southeast-1": "Asia Pacific (Singapore)",
	"ap-southeast-2": "Asia Pacific (Sydney)",
	"sa-east-1":      "South America (Sao Paulo)",
	"me-south-1":     "Middle East (Bahrain)",
	"af-south-1":     "Africa (Cape Town)",
}

// awsLocation resolves a region code to a Price List location name, falling
// back to the default region's location when the code is unknown or empty.
func awsLocation(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		region = awsDefaultRegion
	}
	if loc, ok := awsRegionToLocation[region]; ok {
		return loc
	}
	return awsRegionToLocation[awsDefaultRegion]
}

// awsRegionOrDefault returns the region if set, otherwise the AWS default. It
// centralises the "AWS resources may not carry a region" fallback so mappers
// don't each reimplement it.
func awsRegionOrDefault(region string) string {
	if strings.TrimSpace(region) == "" {
		return awsDefaultRegion
	}
	return region
}

// awsFilter builds one neutral filter entry for a PriceRequest's
// Params["Filters"] list.
func awsFilter(field, value string) map[string]interface{} {
	return map[string]interface{}{"Field": field, "Value": value}
}

// awsPriceRequest assembles a neutral AWS PriceRequest from a service code, a
// region, and a list of attribute filters. The pricing engine routes it to the
// AWS backend by Provider.
func awsPriceRequest(serviceCode, region string, filters ...map[string]interface{}) pricing.PriceRequest {
	list := make([]interface{}, 0, len(filters))
	for _, f := range filters {
		list = append(list, f)
	}
	return pricing.PriceRequest{
		Provider: "aws",
		Product:  serviceCode,
		Region:   awsRegionOrDefault(region),
		Params: map[string]interface{}{
			"Filters":    list,
			"MaxResults": 100,
		},
	}
}

// filterValue reads back the Value of a named filter from a request's
// Params["Filters"] list, used by Parse to label the resulting cost line. It
// returns "" when the filter is absent or malformed.
func filterValue(req pricing.PriceRequest, field string) string {
	if req.Params == nil {
		return ""
	}
	list, ok := req.Params["Filters"].([]interface{})
	if !ok {
		return ""
	}
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if f, _ := m["Field"].(string); f == field {
			v, _ := m["Value"].(string)
			return v
		}
	}
	return ""
}

// awsHourlyToMonthly converts an hourly USD rate into a monthly figure using
// the same 730-hour month the Tencent mappers use, keeping the two providers'
// monthly run-rates directly comparable in magnitude.
func awsHourlyToMonthly(hourlyUSD float64) float64 {
	return hourlyUSD * hoursPerMonth
}

// awsQuantity reads back the Params["Quantity"] a mapper stashed in Extract
// (e.g. EBS provisioned GB, or a usage-driven count), used by Parse to scale a
// per-unit price. Returns 0 when absent.
func awsQuantity(req pricing.PriceRequest) int64 {
	if req.Params == nil {
		return 0
	}
	switch v := req.Params["Quantity"].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}
