package resources

import (
	"math"
	"strings"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

func suppliedItem(quantity float64, unit string, amount, per float64) parser.UsageItem {
	return parser.UsageItem{
		Quantity: quantity,
		Unit:     unit,
		Pricing:  "supplied",
		Rate: parser.UsageRate{
			Amount: amount, Per: per, Currency: "USD",
			Source: parser.UsageSource{
				Kind: "contract", Reference: "contract-42", AsOf: "2026-07-01", Confidence: "high",
			},
		},
	}
}

func TestUsageMappersRegisteredWithOwnedVocabulary(t *testing.T) {
	tests := []struct {
		tfType string
		items  map[string]string
	}{
		{"tencentcloud_cos_bucket", map[string]string{"standard_storage": "GB-month", "get_requests": "request", "put_requests": "request", "internet_egress": "GB"}},
		{"tencentcloud_cdn_domain", map[string]string{"traffic": "GB", "https_requests": "request"}},
		{"tencentcloud_cfs_file_system", map[string]string{"standard_storage": "GB-month"}},
		{"tencentcloud_scf_function", map[string]string{"compute": "GB-second", "invocations": "request", "internet_egress": "GB"}},
		{"aws_s3_bucket", map[string]string{"standard_storage": "GB-month", "get_requests": "request", "put_requests": "request", "internet_egress": "GB"}},
		{"aws_efs_file_system", map[string]string{"standard_storage": "GB-month"}},
		{"aws_eip", map[string]string{"address_hours": "address-hour"}},
	}

	registry := DefaultRegistry()
	for _, tc := range tests {
		t.Run(tc.tfType, func(t *testing.T) {
			mapper, ok := registry.Lookup(tc.tfType)
			if !ok {
				t.Fatalf("%s not registered", tc.tfType)
			}
			usageMapper, ok := mapper.(UsageMapper)
			if !ok {
				t.Fatalf("%s mapper %T does not implement UsageMapper", tc.tfType, mapper)
			}
			items := map[string]parser.UsageItem{}
			for id, unit := range tc.items {
				items[id] = suppliedItem(1, unit, 1, 1)
			}
			components, err := usageMapper.EstimateUsage(parser.PlannedResource{Type: tc.tfType}, parser.UsageResource{Items: items})
			if err != nil {
				t.Fatalf("EstimateUsage: %v", err)
			}
			if len(components) != len(tc.items) {
				t.Fatalf("components = %d, want %d", len(components), len(tc.items))
			}
		})
	}
}

func TestUsageMapperSuppliedRateArithmetic(t *testing.T) {
	mapper, _ := DefaultRegistry().Lookup("aws_s3_bucket")
	components, err := mapper.(UsageMapper).EstimateUsage(parser.PlannedResource{Type: "aws_s3_bucket"}, parser.UsageResource{Items: map[string]parser.UsageItem{
		"standard_storage": suppliedItem(250, "GB-month", 2.5, 100),
		"get_requests":     suppliedItem(0, "request", 0, 1000),
		"put_requests":     suppliedItem(0, "request", 0, 1000),
		"internet_egress":  suppliedItem(0, "GB", 0, 1),
	}})
	if err != nil {
		t.Fatalf("EstimateUsage: %v", err)
	}
	costs := map[string]float64{}
	for _, component := range components {
		costs[component.Name] = component.MonthlyCost
		if component.Currency != "USD" {
			t.Errorf("component currency = %q, want USD", component.Currency)
		}
	}
	if math.Abs(costs["standard_storage"]-6.25) > 1e-9 {
		t.Fatalf("standard_storage monthly = %v, want 6.25", costs["standard_storage"])
	}
	if value, ok := costs["get_requests"]; !ok || value != 0 {
		t.Fatalf("explicit zero component = %v, present = %v", value, ok)
	}
	zero := components[0]
	for _, component := range components {
		if component.Name == "get_requests" {
			zero = component
		}
	}
	if zero.Usage == nil || zero.Usage.Quantity != 0 || zero.Usage.Unit != "request" {
		t.Fatalf("zero usage evidence = %+v", zero.Usage)
	}
	if zero.Rate == nil || zero.Rate.Amount != 0 || zero.Rate.Per != 1000 || zero.Rate.Currency != "USD" {
		t.Fatalf("zero rate evidence = %+v", zero.Rate)
	}
	if zero.Provenance == nil || zero.Provenance.Kind != "user_rate_estimate" {
		t.Fatalf("provenance = %+v", zero.Provenance)
	}
	if zero.Provenance.Source.Kind != "contract" || zero.Provenance.Source.Reference != "contract-42" {
		t.Fatalf("provenance source = %+v", zero.Provenance.Source)
	}
}

func TestUsageMapperRequiresExplicitValueForEveryModeledDimension(t *testing.T) {
	mapper, _ := DefaultRegistry().Lookup("aws_s3_bucket")
	_, err := mapper.(UsageMapper).EstimateUsage(parser.PlannedResource{Type: "aws_s3_bucket"}, parser.UsageResource{Items: map[string]parser.UsageItem{
		"standard_storage": suppliedItem(250, "GB-month", 2.5, 100),
	}})
	if err == nil || !strings.Contains(err.Error(), "supply an explicit zero") {
		t.Fatalf("error = %v, want explicit-zero requirement for omitted dimensions", err)
	}
}

func TestUsageMapperRejectsOverflowedMonthlyCost(t *testing.T) {
	mapper, _ := DefaultRegistry().Lookup("aws_eip")
	_, err := mapper.(UsageMapper).EstimateUsage(parser.PlannedResource{Type: "aws_eip"}, parser.UsageResource{Items: map[string]parser.UsageItem{
		"address_hours": suppliedItem(1e308, "address-hour", 1e308, 1),
	}})
	if err == nil || !strings.Contains(err.Error(), "non-finite monthly cost") {
		t.Fatalf("error = %v, want non-finite monthly cost rejection", err)
	}
}

func TestUsageMapperRejectsUnsupportedItemUnitAndPricing(t *testing.T) {
	mapper, _ := DefaultRegistry().Lookup("aws_eip")
	usageMapper := mapper.(UsageMapper)
	tests := []struct {
		name string
		item map[string]parser.UsageItem
		want string
	}{
		{"item", map[string]parser.UsageItem{"internet_egress": suppliedItem(1, "GB", 1, 1)}, "unsupported usage item"},
		{"unit", map[string]parser.UsageItem{"address_hours": suppliedItem(1, "hour", 1, 1)}, "unit"},
		{"pricing", map[string]parser.UsageItem{"address_hours": func() parser.UsageItem {
			item := suppliedItem(1, "address-hour", 1, 1)
			item.Pricing = "provider"
			return item
		}()}, "pricing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := usageMapper.EstimateUsage(parser.PlannedResource{Type: "aws_eip"}, parser.UsageResource{Items: tc.item})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
