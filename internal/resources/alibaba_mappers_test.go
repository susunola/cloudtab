package resources

import (
	"math"
	"testing"

	"github.com/susunola/cloudtab/internal/pricing"
)

func almostEqualAlibaba(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

const alibabaMockJSON = `{"Code":"Success","Message":"successful","Success":true,"Data":{"Currency":"CNY","ModuleDetails":{"ModuleDetail":[{"InvoiceDiscount":0.96,"UnitPrice":0.96,"CostAfterDiscount":0.96,"OriginalCost":1.0,"ModuleCode":"InstanceType"}]}}}`

func TestAlibabaECSParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaECS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba ECS" {
		t.Errorf("Name = %q, want Alibaba ECS", c.Name)
	}
	if c.Unit != "HOUR" {
		t.Errorf("Unit = %q, want HOUR", c.Unit)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaDiskParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaDisk{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba Disk" {
		t.Errorf("Name = %q, want Alibaba Disk", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaEIPParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaEIP{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba EIP" {
		t.Errorf("Name = %q, want Alibaba EIP", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaSLBParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaSLB{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba SLB" {
		t.Errorf("Name = %q, want Alibaba SLB", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaRDSParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaRDS{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba RDS" {
		t.Errorf("Name = %q, want Alibaba RDS", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaRedisParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaRedis{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba Redis" {
		t.Errorf("Name = %q, want Alibaba Redis", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaMongoDBParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaMongoDB{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba MongoDB" {
		t.Errorf("Name = %q, want Alibaba MongoDB", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaNATParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaNAT{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba NAT Gateway" {
		t.Errorf("Name = %q, want Alibaba NAT Gateway", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaVPNParse(t *testing.T) {
	raw := []byte(alibabaMockJSON)
	comps, err := AlibabaVPN{}.Parse(pricing.PriceRequest{}, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Name != "Alibaba VPN Gateway" {
		t.Errorf("Name = %q, want Alibaba VPN Gateway", c.Name)
	}
	if !almostEqualAlibaba(c.HourlyCost, 0.96) {
		t.Errorf("HourlyCost = %v, want 0.96", c.HourlyCost)
	}
	if !almostEqualAlibaba(c.MonthlyCost, 0.96*hoursPerMonth) {
		t.Errorf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.96*hoursPerMonth)
	}
	if c.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", c.Currency)
	}
}

func TestAlibabaParseNoData(t *testing.T) {
	raw := []byte(`{}`)
	comps, err := AlibabaECS{}.Parse(pricing.PriceRequest{}, raw)
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
