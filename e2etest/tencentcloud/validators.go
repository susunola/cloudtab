package main

// All 11 product validators. Each validator mirrors a Mapper's Parse function
// to extract price fields from the raw API response and compare them with
// cloudtab's CostComponents.
//
// COUPLING NOTE: Validators intentionally duplicate the Mapper's price
// extraction logic. When a Mapper's Parse changes, update the corresponding
// validator. This is the only way to catch "read wrong field" bugs (e.g.
// reading UnitPrice instead of UnitPriceDiscount).

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/pricing"
)

const (
	hoursPerMonth = 730.0
	daysPerMonth  = hoursPerMonth / 24
	tolerance     = 0.01 // abs(api - cloudtab) < 0.01
)

// CheckResult records one validation check.
type CheckResult struct {
	Name     string  `json:"name"`
	APIValue float64 `json:"api_value"`
	GotValue float64 `json:"got_value"`
	Formula  string  `json:"formula,omitempty"`
	Status   string  `json:"status"` // PASS | SUSPICIOUS | FAIL
}

// Validator validates a single resource's price against the raw API response.
type Validator interface {
	Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult
}

// abs returns the absolute value.
func abs(f float64) float64 { return math.Abs(f) }

// almostEq checks if two floats are within tolerance.
func almostEq(a, b float64) bool { return abs(a-b) < tolerance }

// checkPreZero returns SUSPICIOUS if the API price is 0 (possible Response
// wrapper bug), otherwise returns an empty CheckResult (pass-through).
func checkPreZero(name string, apiVal float64) *CheckResult {
	if apiVal == 0 {
		return &CheckResult{
			Name:     name + " (>0 precondition)",
			APIValue: 0,
			Status:   "SUSPICIOUS",
			Formula:  "API price is 0 — possible Response wrapper parse bug",
		}
	}
	return nil
}

// passCheck creates a PASS CheckResult.
func passCheck(name string, api, got float64, formula string) CheckResult {
	return CheckResult{Name: name, APIValue: api, GotValue: got, Formula: formula, Status: "PASS"}
}

// failCheck creates a FAIL CheckResult.
func failCheck(name string, api, got float64, formula string) CheckResult {
	return CheckResult{Name: name, APIValue: api, GotValue: got, Formula: formula, Status: "FAIL"}
}

// compareVal compares API and cloudtab values, returning PASS or FAIL.
func compareVal(name string, api, got float64, formula string) CheckResult {
	if almostEq(api, got) {
		return passCheck(name, api, got, formula)
	}
	return failCheck(name, api, got, formula)
}

// ============================================================
// Validator 1: cvmClbValidator — CVM, CLB
// Mirrors: CVMInstance.Parse / CLBInstance.Parse
// Path: Response.Price.InstancePrice.{UnitPrice, UnitPriceDiscount, DiscountPrice, ChargeUnit}
// Unit: CNY
// ============================================================

type cvmClbValidator struct{}

func (v cvmClbValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type itemPrice struct {
		UnitPrice         float64 `json:"UnitPrice"`
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		OriginalPrice     float64 `json:"OriginalPrice"`
		DiscountPrice     float64 `json:"DiscountPrice"`
		ChargeUnit        string  `json:"ChargeUnit"`
	}
	var wrap struct {
		Price struct {
			InstancePrice itemPrice `json:"InstancePrice"`
		} `json:"Price"`
		Response struct {
			Price struct {
				InstancePrice itemPrice `json:"InstancePrice"`
			} `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	ip := wrap.Price.InstancePrice
	if wrap.Response.Price.InstancePrice.UnitPrice > 0 || wrap.Response.Price.InstancePrice.UnitPriceDiscount > 0 ||
		wrap.Response.Price.InstancePrice.DiscountPrice > 0 {
		ip = wrap.Response.Price.InstancePrice
	}

	prepaid := isPrepaid(req)
	var results []CheckResult

	if prepaid {
		apiMonthly := ip.DiscountPrice
		if susp := checkPreZero("DiscountPrice", apiMonthly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("DiscountPrice=%.2f", apiMonthly)))
		if len(comps) > 0 {
			results = append(results, compareVal("monthly = DiscountPrice", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f", apiMonthly)))
		}
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = 0 (PREPAID)", 0, comps[0].HourlyCost, "PREPAID no hourly"))
		}
	} else {
		apiHourly := ip.UnitPriceDiscount
		if susp := checkPreZero("UnitPriceDiscount", apiHourly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiHourly, 0, fmt.Sprintf("UnitPriceDiscount=%.4f", apiHourly)))
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = UnitPriceDiscount", apiHourly, comps[0].HourlyCost, fmt.Sprintf("%.4f", apiHourly)))
			wantMonthly := apiHourly * hoursPerMonth
			results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", apiHourly, wantMonthly)))
		}
	}
	return results
}

// ============================================================
// Validator 2: cbsValidator — CBS
// Mirrors: CBSStorage.Parse
// Path: Response.DiskPrice.{UnitPrice, UnitPriceDiscount, DiscountPrice, ChargeUnit}
// Unit: CNY
// ============================================================

type cbsValidator struct{}

func (v cbsValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type diskPrice struct {
		UnitPrice         float64 `json:"UnitPrice"`
		UnitPriceDiscount float64 `json:"UnitPriceDiscount"`
		DiscountPrice     float64 `json:"DiscountPrice"`
		ChargeUnit        string  `json:"ChargeUnit"`
	}
	var wrap struct {
		DiskPrice diskPrice `json:"DiskPrice"`
		Response  struct {
			DiskPrice diskPrice `json:"DiskPrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	dp := wrap.DiskPrice
	if wrap.Response.DiskPrice.UnitPrice > 0 || wrap.Response.DiskPrice.UnitPriceDiscount > 0 ||
		wrap.Response.DiskPrice.DiscountPrice > 0 {
		dp = wrap.Response.DiskPrice
	}

	// CBS is always POSTPAID_BY_HOUR in the default test config
	apiHourly := dp.UnitPriceDiscount
	var results []CheckResult
	if susp := checkPreZero("UnitPriceDiscount", apiHourly); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", apiHourly, 0, fmt.Sprintf("UnitPriceDiscount=%.4f", apiHourly)))
	if len(comps) > 0 {
		results = append(results, compareVal("hourly = UnitPriceDiscount", apiHourly, comps[0].HourlyCost, fmt.Sprintf("%.4f", apiHourly)))
		wantMonthly := apiHourly * hoursPerMonth
		results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", apiHourly, wantMonthly)))
	}
	return results
}

// ============================================================
// Validator 3: cdbFenValidator — MySQL, PostgreSQL, MariaDB, SQLServer, DCDB, CWP, CloudHSM
// Mirrors: respective Parse functions (all use parseTencentPrice: Response.{Price, OriginalPrice})
// Unit: cents (/100 -> CNY)
// ============================================================

type cdbFenValidator struct{}

func (v cdbFenValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	// parseTencentPrice: Response.{Price, OriginalPrice} in cents
	var wrap struct {
		Price    float64 `json:"Price"`
		Original float64 `json:"OriginalPrice"`
		Response struct {
			Price    float64 `json:"Price"`
			Original float64 `json:"OriginalPrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	price := wrap.Price
	if wrap.Response.Price > 0 || wrap.Response.Original > 0 {
		price = wrap.Response.Price
		if price == 0 {
			price = wrap.Response.Original
		}
	}
	if price == 0 {
		price = wrap.Original
	}

	priceYuan := price / 100.0
	prepaid := isPrepaid(req)
	var results []CheckResult

	if susp := checkPreZero("Price (cents)", price); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", price, 0, fmt.Sprintf("Price=%.0fcents -> %.2fCNY", price, priceYuan)))

	if prepaid {
		if len(comps) > 0 {
			results = append(results, compareVal("monthly = Price/100", priceYuan, comps[0].MonthlyCost, fmt.Sprintf("%.0f/100=%.2f", price, priceYuan)))
			results = append(results, compareVal("hourly = 0 (PREPAID)", 0, comps[0].HourlyCost, "PREPAID no hourly"))
		}
	} else {
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = Price/100", priceYuan, comps[0].HourlyCost, fmt.Sprintf("%.0f/100=%.4f", price, priceYuan)))
			wantMonthly := priceYuan * hoursPerMonth
			results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", priceYuan, wantMonthly)))
		}
	}
	return results
}

// ============================================================
// Validator 4: redisValidator — Redis
// Mirrors: RedisInstance.Parse
// Path: Response.{Price, HighPrecisionPrice, AmountUnit}
// Unit: cents/micro-cents (normalizeTencentAmount)
// ============================================================

type redisValidator struct{}

func (v redisValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	var wrap struct {
		Price              float64 `json:"Price"`
		HighPrecisionPrice float64 `json:"HighPrecisionPrice"`
		AmountUnit         string  `json:"AmountUnit"`
		Response           struct {
			Price              float64 `json:"Price"`
			HighPrecisionPrice float64 `json:"HighPrecisionPrice"`
			AmountUnit         string  `json:"AmountUnit"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}

	// Prefer Response-wrapped values
	price := wrap.Price
	hp := wrap.HighPrecisionPrice
	unit := wrap.AmountUnit
	if wrap.Response.Price > 0 || wrap.Response.HighPrecisionPrice > 0 {
		price = wrap.Response.Price
		hp = wrap.Response.HighPrecisionPrice
		unit = wrap.Response.AmountUnit
	}

	// Use the same normalizeTencentAmount logic as the mapper:
	// The Redis API returns Price in cents by default (no AmountUnit in response),
	// so default to "pent" -> /100, matching RedisInstance.Parse.
	if strings.TrimSpace(unit) == "" {
		unit = "pent"
	}
	rawPrice := hp
	if rawPrice == 0 {
		rawPrice = price
	}
	priceYuan := rawPrice // default: already in CNY
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "pent":
		priceYuan = rawPrice / 100.0
	case "micropent":
		priceYuan = rawPrice / 100000000.0
	}

	prepaid := isPrepaid(req)
	var results []CheckResult
	if susp := checkPreZero("Price", priceYuan); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", priceYuan, 0, fmt.Sprintf("priceYuan=%.4f", priceYuan)))

	if prepaid {
		if len(comps) > 0 {
			results = append(results, compareVal("monthly = priceYuan", priceYuan, comps[0].MonthlyCost, fmt.Sprintf("%.4f", priceYuan)))
		}
	} else {
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = priceYuan", priceYuan, comps[0].HourlyCost, fmt.Sprintf("%.4f", priceYuan)))
			wantMonthly := priceYuan * hoursPerMonth
			results = append(results, compareVal("monthly = priceYuanx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", priceYuan, wantMonthly)))
		}
	}
	return results
}

// ============================================================
// Validator 5: mongodbValidator — MongoDB
// Mirrors: MongoDBInstance.Parse
// Path: Response.Price.{UnitPrice, OriginalPrice, DiscountPrice}
// Unit: CNY (POSTPAID uses UnitPrice, PREPAID uses DiscountPrice)
// ============================================================

type mongodbValidator struct{}

func (v mongodbValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	var wrap struct {
		Price struct {
			UnitPrice     float64 `json:"UnitPrice"`
			OriginalPrice float64 `json:"OriginalPrice"`
			DiscountPrice float64 `json:"DiscountPrice"`
		} `json:"Price"`
		Response struct {
			Price struct {
				UnitPrice     float64 `json:"UnitPrice"`
				OriginalPrice float64 `json:"OriginalPrice"`
				DiscountPrice float64 `json:"DiscountPrice"`
			} `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	p := wrap.Price
	if wrap.Response.Price.UnitPrice > 0 || wrap.Response.Price.DiscountPrice > 0 {
		p = wrap.Response.Price
	}

	prepaid := isPrepaid(req)
	var results []CheckResult

	if prepaid {
		apiMonthly := p.DiscountPrice
		if susp := checkPreZero("DiscountPrice", apiMonthly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("DiscountPrice=%.2f", apiMonthly)))
		if len(comps) > 0 {
			results = append(results, compareVal("monthly = DiscountPrice", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f", apiMonthly)))
		}
	} else {
		apiHourly := p.UnitPrice
		if susp := checkPreZero("UnitPrice", apiHourly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiHourly, 0, fmt.Sprintf("UnitPrice=%.4f", apiHourly)))
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = UnitPrice", apiHourly, comps[0].HourlyCost, fmt.Sprintf("%.4f", apiHourly)))
			wantMonthly := apiHourly * hoursPerMonth
			results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", apiHourly, wantMonthly)))
		}
	}
	return results
}

// ============================================================
// Validator 6: cynosdbValidator — CynosDB (TDSQL-C)
// Mirrors: CynosDBCluster.Parse
// Path: Response.{InstancePrice, StoragePrice}.{UnitPriceDiscount, TotalPriceDiscount}
// Unit: cents (int64, /100 -> CNY)
// Note: Two components (compute + storage)
// ============================================================

type cynosdbValidator struct{}

func (v cynosdbValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type priceBlock struct {
		UnitPrice          float64 `json:"UnitPrice"`
		UnitPriceDiscount  float64 `json:"UnitPriceDiscount"`
		TotalPrice         float64 `json:"TotalPrice"`
		TotalPriceDiscount float64 `json:"TotalPriceDiscount"`
	}
	var wrap struct {
		InstancePrice priceBlock `json:"InstancePrice"`
		StoragePrice  priceBlock `json:"StoragePrice"`
		Response      struct {
			InstancePrice priceBlock `json:"InstancePrice"`
			StoragePrice  priceBlock `json:"StoragePrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	ip := wrap.InstancePrice
	sp := wrap.StoragePrice
	if wrap.Response.InstancePrice.UnitPriceDiscount > 0 || wrap.Response.InstancePrice.TotalPriceDiscount > 0 {
		ip = wrap.Response.InstancePrice
		sp = wrap.Response.StoragePrice
	}

	prepaid := isPrepaid(req)
	var results []CheckResult

	if prepaid {
		apiMonthly := (ip.TotalPriceDiscount + sp.TotalPriceDiscount) / 100.0
		if susp := checkPreZero("TotalPriceDiscount", apiMonthly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("Instance+Storage=%.0fcents->%.2fCNY", ip.TotalPriceDiscount+sp.TotalPriceDiscount, apiMonthly)))
		// Sum all components' monthly
		var totalMonthly float64
		for _, c := range comps {
			totalMonthly += c.MonthlyCost
		}
		results = append(results, compareVal("monthly = (Instance+Storage)/100", apiMonthly, totalMonthly, fmt.Sprintf("(%.0f+%.0f)/100=%.2f", ip.TotalPriceDiscount, sp.TotalPriceDiscount, apiMonthly)))
	} else {
		apiHourly := ip.UnitPriceDiscount / 100.0
		if susp := checkPreZero("UnitPriceDiscount", apiHourly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiHourly, 0, fmt.Sprintf("UnitPriceDiscount=%.0fcents->%.4fCNY", ip.UnitPriceDiscount, apiHourly)))
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = UnitPriceDiscount/100", apiHourly, comps[0].HourlyCost, fmt.Sprintf("%.0f/100=%.4f", ip.UnitPriceDiscount, apiHourly)))
			wantMonthly := apiHourly * hoursPerMonth
			results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", apiHourly, wantMonthly)))
		}
	}
	return results
}

// ============================================================
// Validator 7: lighthouseValidator — Lighthouse
// Mirrors: LighthouseInstance.Parse
// Path: Response.Price.InstancePrice.{DiscountPrice, OriginalPrice}
// Unit: CNY (PREPAID only, monthly total)
// ============================================================

type lighthouseValidator struct{}

func (v lighthouseValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type itemPrice struct {
		DiscountPrice float64 `json:"DiscountPrice"`
		OriginalPrice float64 `json:"OriginalPrice"`
	}
	var wrap struct {
		Price struct {
			InstancePrice itemPrice `json:"InstancePrice"`
		} `json:"Price"`
		Response struct {
			Price struct {
				InstancePrice itemPrice `json:"InstancePrice"`
			} `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	ip := wrap.Price.InstancePrice
	if wrap.Response.Price.InstancePrice.DiscountPrice > 0 || wrap.Response.Price.InstancePrice.OriginalPrice > 0 {
		ip = wrap.Response.Price.InstancePrice
	}

	apiMonthly := ip.DiscountPrice
	if apiMonthly == 0 {
		apiMonthly = ip.OriginalPrice
	}

	var results []CheckResult
	if susp := checkPreZero("DiscountPrice", apiMonthly); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("DiscountPrice=%.2f", apiMonthly)))
	if len(comps) > 0 {
		results = append(results, compareVal("monthly = DiscountPrice", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f", apiMonthly)))
		results = append(results, compareVal("hourly = 0 (PREPAID)", 0, comps[0].HourlyCost, "PREPAID no hourly"))
	}
	return results
}

// ============================================================
// Validator 8: ecmValidator — ECM
// Mirrors: ECMInstance.Parse
// Path: Response.InstancePrice.{DiscountPrice, OriginalPrice}
// Unit: cents (uint64, /100 -> CNY), POSTPAID only
// ============================================================

type ecmValidator struct{}

func (v ecmValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type itemPrice struct {
		DiscountPrice float64 `json:"DiscountPrice"`
		OriginalPrice float64 `json:"OriginalPrice"`
	}
	var wrap struct {
		InstancePrice itemPrice `json:"InstancePrice"`
		Response      struct {
			InstancePrice itemPrice `json:"InstancePrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	ip := wrap.InstancePrice
	if wrap.Response.InstancePrice.DiscountPrice > 0 || wrap.Response.InstancePrice.OriginalPrice > 0 {
		ip = wrap.Response.InstancePrice
	}

	apiHourly := ip.DiscountPrice / 100.0
	if apiHourly == 0 {
		apiHourly = ip.OriginalPrice / 100.0
	}

	var results []CheckResult
	if susp := checkPreZero("DiscountPrice", apiHourly); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", apiHourly, 0, fmt.Sprintf("DiscountPrice=%.0fcents->%.4fCNY", ip.DiscountPrice, apiHourly)))
	if len(comps) > 0 {
		results = append(results, compareVal("hourly = DiscountPrice/100", apiHourly, comps[0].HourlyCost, fmt.Sprintf("%.0f/100=%.4f", ip.DiscountPrice, apiHourly)))
		wantMonthly := apiHourly * hoursPerMonth
		results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", apiHourly, wantMonthly)))
	}
	return results
}

// ============================================================
// Validator 9: gaapValidator — GAAP
// Mirrors: GAAPProxy.Parse
// Path: Response.{ProxyDailyPrice, DiscountProxyDailyPrice}
// Unit: CNY/day (x daysPerMonth -> CNY/month)
// ============================================================

type gaapValidator struct{}

func (v gaapValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	var wrap struct {
		ProxyDailyPrice         float64 `json:"ProxyDailyPrice"`
		DiscountProxyDailyPrice float64 `json:"DiscountProxyDailyPrice"`
		Response                struct {
			ProxyDailyPrice         float64 `json:"ProxyDailyPrice"`
			DiscountProxyDailyPrice float64 `json:"DiscountProxyDailyPrice"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	daily := wrap.DiscountProxyDailyPrice
	if daily == 0 {
		daily = wrap.ProxyDailyPrice
	}
	if wrap.Response.DiscountProxyDailyPrice > 0 || wrap.Response.ProxyDailyPrice > 0 {
		daily = wrap.Response.DiscountProxyDailyPrice
		if daily == 0 {
			daily = wrap.Response.ProxyDailyPrice
		}
	}

	apiMonthly := daily * daysPerMonth
	var results []CheckResult
	if susp := checkPreZero("DiscountProxyDailyPrice", daily); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", daily, 0, fmt.Sprintf("daily=%.4f->monthly=%.2f", daily, apiMonthly)))
	if len(comps) > 0 {
		results = append(results, compareVal("monthly = dailyPricexdaysPerMonth", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx%.1f=%.2f", daily, daysPerMonth, apiMonthly)))
	}
	return results
}

// ============================================================
// Validator 10: vpnValidator — VPN Gateway
// Mirrors: VPNGateway.Parse
// Path: Response.Price.{InstancePrice, BandwidthPrice}.{UnitPrice, DiscountPrice, ChargeUnit}
// Unit: CNY (POSTPAID uses UnitPrice — NOT UnitPriceDiscount!)
// ============================================================

type vpnValidator struct{}

func (v vpnValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type itemPrice struct {
		UnitPrice     float64 `json:"UnitPrice"`
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
		ChargeUnit    string  `json:"ChargeUnit"`
	}
	var wrap struct {
		Price struct {
			InstancePrice  itemPrice `json:"InstancePrice"`
			BandwidthPrice itemPrice `json:"BandwidthPrice"`
		} `json:"Price"`
		Response struct {
			Price struct {
				InstancePrice  itemPrice `json:"InstancePrice"`
				BandwidthPrice itemPrice `json:"BandwidthPrice"`
			} `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	ip := wrap.Price.InstancePrice
	if wrap.Response.Price.InstancePrice.UnitPrice > 0 || wrap.Response.Price.InstancePrice.DiscountPrice > 0 {
		ip = wrap.Response.Price.InstancePrice
	}

	prepaid := isPrepaid(req)
	var results []CheckResult

	if prepaid {
		apiMonthly := ip.DiscountPrice
		if apiMonthly == 0 {
			apiMonthly = ip.OriginalPrice
		}
		if susp := checkPreZero("DiscountPrice", apiMonthly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("DiscountPrice=%.2f", apiMonthly)))
		if len(comps) > 0 {
			results = append(results, compareVal("monthly = DiscountPrice", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f", apiMonthly)))
		}
	} else {
		// POSTPAID: use DiscountPrice (actual cost), fall back to UnitPrice.
		apiHourly := ip.DiscountPrice
		if apiHourly == 0 {
			apiHourly = ip.UnitPrice
		}
		if susp := checkPreZero("DiscountPrice/UnitPrice", apiHourly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiHourly, 0, fmt.Sprintf("DiscountPrice=%.4f", apiHourly)))
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = DiscountPrice", apiHourly, comps[0].HourlyCost, fmt.Sprintf("%.4f", apiHourly)))
			wantMonthly := apiHourly * hoursPerMonth
			results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", apiHourly, wantMonthly)))
		}
	}
	return results
}

// ============================================================
// Validator 12: eipValidator — EIP
// Mirrors: EIP.Parse
// Path: Response.Price.AddressPrice.{UnitPrice, DiscountPrice, OriginalPrice, ChargeUnit}
// Unit: CNY (POSTPAID: HOUR rate; PREPAID: total for period)
// ============================================================

type eipValidator struct{}

func (v eipValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type addressPrice struct {
		UnitPrice     float64 `json:"UnitPrice"`
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
		ChargeUnit    string  `json:"ChargeUnit"`
	}
	var wrap struct {
		Price struct {
			AddressPrice addressPrice `json:"AddressPrice"`
		} `json:"Price"`
		Response struct {
			Price struct {
				AddressPrice addressPrice `json:"AddressPrice"`
			} `json:"Price"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}

	ap := wrap.Price.AddressPrice
	if wrap.Response.Price.AddressPrice.UnitPrice > 0 ||
		wrap.Response.Price.AddressPrice.DiscountPrice > 0 ||
		wrap.Response.Price.AddressPrice.OriginalPrice > 0 {
		ap = wrap.Response.Price.AddressPrice
	}

	prepaid := isPrepaid(req)
	var results []CheckResult

	if prepaid {
		apiMonthly := ap.DiscountPrice
		if apiMonthly == 0 {
			apiMonthly = ap.OriginalPrice
		}
		if susp := checkPreZero("DiscountPrice", apiMonthly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("DiscountPrice=%.2f", apiMonthly)))
		if len(comps) > 0 {
			results = append(results, compareVal("monthly = DiscountPrice", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f", apiMonthly)))
		}
	} else {
		apiHourly := ap.DiscountPrice
		if apiHourly == 0 {
			apiHourly = ap.UnitPrice
		}
		if susp := checkPreZero("DiscountPrice", apiHourly); susp != nil {
			results = append(results, *susp)
			return results
		}
		results = append(results, passCheck("API price > 0 precondition", apiHourly, 0, fmt.Sprintf("DiscountPrice=%.4f", apiHourly)))
		if len(comps) > 0 {
			results = append(results, compareVal("hourly = DiscountPrice", apiHourly, comps[0].HourlyCost, fmt.Sprintf("%.4f", apiHourly)))
			wantMonthly := apiHourly * hoursPerMonth
			results = append(results, compareVal("monthly = hourlyx730", wantMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4fx730=%.2f", apiHourly, wantMonthly)))
		}
	}
	return results
}

// ============================================================
// Validator: dcgValidator — Direct Connect Gateway
// Mirrors: DirectConnectGateway.Parse
// Path: Response.{TotalCost, RealTotalCost}
// Unit: CNY (int64, always PREPAID monthly, no hourly)
// ============================================================

type dcgValidator struct{}

func (v dcgValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	var wrap struct {
		TotalCost     float64 `json:"TotalCost"`
		RealTotalCost float64 `json:"RealTotalCost"`
		Response      struct {
			TotalCost     float64 `json:"TotalCost"`
			RealTotalCost float64 `json:"RealTotalCost"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}

	cost := wrap.RealTotalCost
	if cost == 0 {
		cost = wrap.Response.RealTotalCost
	}
	if cost == 0 {
		cost = wrap.TotalCost
	}
	if cost == 0 {
		cost = wrap.Response.TotalCost
	}

	apiMonthly := cost
	var results []CheckResult
	if susp := checkPreZero("RealTotalCost", apiMonthly); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("RealTotalCost=%.4fCNY", cost)))
	if len(comps) > 0 {
		results = append(results, compareVal("monthly = RealTotalCost", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.4f", cost)))
	}
	return results
}

// ============================================================
// Validator 12: cwpValidator — CWP (YunjingLicense)
// Mirrors: YunjingLicense.Parse
// Path: Response.{OriginalPrice, DiscountPrice}
// Unit: CNY (NOT cents! No /100 conversion)
// Always PREPAID (monthly license, no hourly)
// ============================================================

type cwpValidator struct{}

func (v cwpValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type priceBlock struct {
		OriginalPrice float64 `json:"OriginalPrice"`
		DiscountPrice float64 `json:"DiscountPrice"`
	}
	var wrap struct {
		priceBlock
		Response struct {
			priceBlock
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	pb := wrap.priceBlock
	if wrap.Response.OriginalPrice > 0 || wrap.Response.DiscountPrice > 0 {
		pb = wrap.Response.priceBlock
	}

	// preferDiscount: use DiscountPrice if > 0, else OriginalPrice. Values are CNY.
	apiMonthly := pb.DiscountPrice
	if apiMonthly == 0 {
		apiMonthly = pb.OriginalPrice
	}

	var results []CheckResult
	if susp := checkPreZero("DiscountPrice", apiMonthly); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("DiscountPrice=%.2fCNY", apiMonthly)))
	if len(comps) > 0 {
		results = append(results, compareVal("monthly = preferDiscount(CNY)", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f", apiMonthly)))
		results = append(results, compareVal("hourly = 0 (PREPAID)", 0, comps[0].HourlyCost, "PREPAID no hourly"))
	}
	return results
}

// ============================================================
// Validator 13: cloudhsmValidator — CloudHSM
// Mirrors: CloudHSMInstance.Parse
// Path: Response.{TotalCost, OriginalCost}
// Unit: CNY (NOT cents! No /100 conversion)
// Always PREPAID (monthly, no hourly)
// Note: Field names are TotalCost/OriginalCost, NOT Price/OriginalPrice
// ============================================================

type cloudhsmValidator struct{}

func (v cloudhsmValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type priceBlock struct {
		TotalCost    *float64 `json:"TotalCost"`
		OriginalCost *float64 `json:"OriginalCost"`
	}
	var wrap struct {
		priceBlock
		Response struct {
			priceBlock
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	pb := wrap.priceBlock
	if wrap.Response.TotalCost != nil || wrap.Response.OriginalCost != nil {
		pb = wrap.Response.priceBlock
	}

	apiMonthly := 0.0
	if pb.TotalCost != nil && *pb.TotalCost > 0 {
		apiMonthly = *pb.TotalCost
	} else if pb.OriginalCost != nil {
		apiMonthly = *pb.OriginalCost
	}

	var results []CheckResult
	if susp := checkPreZero("TotalCost", apiMonthly); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", apiMonthly, 0, fmt.Sprintf("TotalCost=%.2fCNY", apiMonthly)))
	if len(comps) > 0 {
		results = append(results, compareVal("monthly = TotalCost(CNY)", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f", apiMonthly)))
		results = append(results, compareVal("hourly = 0 (PREPAID)", 0, comps[0].HourlyCost, "PREPAID no hourly"))
	}
	return results
}

// ============================================================
// Validator 12: domainValidator — Domain Registration
// Mirrors: DomainRegistration.Parse
// Path: Response.PriceList[].{RealPrice, ...}
// Unit: CNY/year (/12 -> CNY/month)
// ============================================================

type domainValidator struct{}

func (v domainValidator) Validate(req pricing.PriceRequest, raw []byte, comps []output.CostComponent) []CheckResult {
	type priceInfo struct {
		RealPrice float64 `json:"RealPrice"`
		Price     float64 `json:"Price"`
	}
	var wrap struct {
		PriceList []priceInfo `json:"PriceList"`
		Response  struct {
			PriceList []priceInfo `json:"PriceList"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return []CheckResult{{Name: "parse response", Status: "FAIL", Formula: err.Error()}}
	}
	pl := wrap.PriceList
	if len(wrap.Response.PriceList) > 0 {
		pl = wrap.Response.PriceList
	}

	var results []CheckResult
	if len(pl) == 0 {
		results = append(results, failCheck("PriceList non-empty", 1, 0, "empty PriceList"))
		return results
	}
	yearly := pl[0].RealPrice
	if yearly == 0 {
		yearly = pl[0].Price
	}
	apiMonthly := yearly / 12.0

	if susp := checkPreZero("RealPrice", yearly); susp != nil {
		results = append(results, *susp)
		return results
	}
	results = append(results, passCheck("API price > 0 precondition", yearly, 0, fmt.Sprintf("RealPrice=%.2f/year->%.2f/month", yearly, apiMonthly)))
	if len(comps) > 0 {
		results = append(results, compareVal("monthly = RealPrice/12", apiMonthly, comps[0].MonthlyCost, fmt.Sprintf("%.2f/12=%.2f", yearly, apiMonthly)))
	}
	return results
}
