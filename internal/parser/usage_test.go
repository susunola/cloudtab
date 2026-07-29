package parser

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsageYAMLEmptyPath(t *testing.T) {
	u, err := LoadUsageYAML("")
	if err != nil {
		t.Fatalf("LoadUsageYAML(\"\") error = %v", err)
	}
	if len(u) != 0 {
		t.Fatalf("len = %d, want 0", len(u))
	}
}

func TestLoadUsageYAML(t *testing.T) {
	doc := `
 tencentcloud_cos_bucket.static_site:
   monthly_storage_gb: 200
   monthly_get_requests: 1000000
 tencentcloud_redis_instance.cache:
   mem_size: 4096
 `
	p := filepath.Join(t.TempDir(), "usage.yml")
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	u, err := LoadUsageYAML(p)
	if err != nil {
		t.Fatalf("LoadUsageYAML() error = %v", err)
	}
	if got := u["tencentcloud_cos_bucket.static_site"]["monthly_storage_gb"]; got != 200 {
		t.Fatalf("monthly_storage_gb = %v, want 200", got)
	}
	if got := u["tencentcloud_redis_instance.cache"]["mem_size"]; got != 4096 {
		t.Fatalf("mem_size = %v, want 4096", got)
	}
}

func TestLoadUsageYAMLBadPath(t *testing.T) {
	if _, err := LoadUsageYAML(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("expected error for missing usage file")
	}
}

func writeUsageFile(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.yml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadUsageFileVersionOne(t *testing.T) {
	usage, err := LoadUsageFile(writeUsageFile(t, `
version: 1
resources:
  aws_s3_bucket.assets:
    items:
      standard_storage:
        quantity: 0
        unit: GB-month
        pricing: supplied
        rate:
          amount: 0
          per: 1
          currency: usd
          source:
            kind: contract
            reference: contract-42
            as_of: 2026-07-01
            confidence: high
`))
	if err != nil {
		t.Fatalf("LoadUsageFile: %v", err)
	}
	if !usage.IsVersioned() || usage.Version != 1 {
		t.Fatalf("usage version = %d, versioned = %v", usage.Version, usage.IsVersioned())
	}
	if len(usage.Legacy) != 0 {
		t.Fatalf("versioned usage populated legacy overrides: %#v", usage.Legacy)
	}
	item := usage.Resources["aws_s3_bucket.assets"].Items["standard_storage"]
	if item.Quantity != 0 || item.Rate.Amount != 0 || item.Rate.Per != 1 {
		t.Fatalf("item numeric fields = %+v", item)
	}
	if item.Rate.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", item.Rate.Currency)
	}
	if item.Rate.Source.Kind != "contract" || item.Rate.Source.AsOf != "2026-07-01" {
		t.Fatalf("source = %+v", item.Rate.Source)
	}
}

func TestLoadUsageFileKeepsLegacyOverrides(t *testing.T) {
	usage, err := LoadUsageFile(writeUsageFile(t, `
tencentcloud_redis_instance.cache:
  mem_size: 4096
`))
	if err != nil {
		t.Fatalf("LoadUsageFile: %v", err)
	}
	if usage.IsVersioned() {
		t.Fatal("legacy file detected as versioned")
	}
	if got := usage.Legacy["tencentcloud_redis_instance.cache"]["mem_size"]; got != 4096 {
		t.Fatalf("mem_size = %v, want 4096", got)
	}
	if len(usage.Resources) != 0 {
		t.Fatalf("legacy usage populated typed resources: %#v", usage.Resources)
	}
}

func TestLoadUsageFileRejectsInvalidVersionOne(t *testing.T) {
	validItem := `
        quantity: 1
        unit: GB-month
        pricing: supplied
        rate:
          amount: 2
          per: 1
          currency: USD
          source:
            kind: provider_documentation
            reference: https://example.invalid/rate
            as_of: 2026-07-01
            confidence: high`
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"unsupported version", "version: 2\nresources: {}\n", "unsupported usage version"},
		{"multiple documents", "version: 1\nresources: {}\n---\nversion: 1\nresources: {}\n", "multiple YAML documents"},
		{"unknown root field", "version: 1\nresources: {}\nextra: true\n", "field extra not found"},
		{"unknown item field", "version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:" + validItem + "\n        extra: true\n", "field extra not found"},
		{"negative quantity", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "quantity: 1", "quantity: -1", 1), "quantity"},
		{"nan quantity", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "quantity: 1", "quantity: .nan", 1), "quantity"},
		{"infinite rate", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "amount: 2", "amount: .inf", 1), "amount"},
		{"zero per", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "per: 1", "per: 0", 1), "per"},
		{"missing source", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "          source:\n            kind: provider_documentation\n            reference: https://example.invalid/rate\n            as_of: 2026-07-01\n            confidence: high", "", 1), "source"},
		{"bad source kind", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "provider_documentation", "guess", 1), "source kind"},
		{"bad date", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "2026-07-01", "2026-02-30", 1), "as_of"},
		{"bad confidence", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "confidence: high", "confidence: certain", 1), "confidence"},
		{"empty currency", strings.Replace("version: 1\nresources:\n  aws_s3_bucket.x:\n    items:\n      standard_storage:"+validItem+"\n", "currency: USD", "currency: ' '", 1), "currency"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadUsageFile(writeUsageFile(t, tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestUsageNumbersRemainFinite(t *testing.T) {
	usage, err := LoadUsageFile(writeUsageFile(t, `
version: 1
resources:
  aws_eip.web:
    items:
      address_hours:
        quantity: 730
        unit: address-hour
        pricing: supplied
        rate:
          amount: 0.005
          per: 1
          currency: USD
          source:
            kind: historical_bill
            reference: invoice-2026-06
            as_of: 2026-06-30
            confidence: medium
`))
	if err != nil {
		t.Fatal(err)
	}
	item := usage.Resources["aws_eip.web"].Items["address_hours"]
	if math.IsNaN(item.Quantity) || math.IsInf(item.Quantity, 0) || math.IsNaN(item.Rate.Amount) || math.IsInf(item.Rate.Amount, 0) {
		t.Fatalf("non-finite parsed item: %+v", item)
	}
}
