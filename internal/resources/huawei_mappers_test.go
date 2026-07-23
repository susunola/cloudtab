package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/pricing"
)

func almostEqualHuawei(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

const huaweiMockJSON = `{"amount":2.45,"official_website_amount":2.50,"measure_id":1,"currency":"CNY"}`

func TestHuaweiECSParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiECS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei ECS" {
		t.Errorf("Name = %q, want Huawei ECS", c.Name)
	}
	if c.Unit != "HOUR" {
		t.Errorf("Unit = %q, want HOUR", c.Unit)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiEVSParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiEVS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei EVS" {
		t.Errorf("Name = %q, want Huawei EVS", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiEIPParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiEIP{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei EIP" {
		t.Errorf("Name = %q, want Huawei EIP", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiELBParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiELB{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei ELB" {
		t.Errorf("Name = %q, want Huawei ELB", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiRDSParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiRDS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei RDS" {
		t.Errorf("Name = %q, want Huawei RDS", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiDCSParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiDCS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei DCS" {
		t.Errorf("Name = %q, want Huawei DCS", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiDDSParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiDDS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei DDS" {
		t.Errorf("Name = %q, want Huawei DDS", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiNATParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiNAT{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei NAT Gateway" {
		t.Errorf("Name = %q, want Huawei NAT Gateway", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiCCEParse(t *testing.T) {
	raw := []byte(huaweiMockJSON)
	comps, err := HuaweiCCE{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Huawei CCE" {
		t.Errorf("Name = %q, want Huawei CCE", c.Name)
	}
	if !almostEqualHuawei(c.HourlyCost, 2.45) {
		t.Errorf("HourlyCost = %v, want 2.45", c.HourlyCost)
	}
	if !almostEqualHuawei(c.MonthlyCost, 2.45*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 2.45*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestHuaweiParseNoData(t *testing.T) {
	raw := []byte(`{}`)
	comps, err := HuaweiECS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.HourlyCost != 0 {
		t.Errorf("HourlyCost = %v, want 0 (no data)", c.HourlyCost)
	}
	if c.MonthlyCost != 0 {
		t.Errorf("MonthlyCost = %v, want 0 (no data)", c.MonthlyCost)
	}
}

// keep the pricing import referenced
var _ = pricing.PriceRequest{}
