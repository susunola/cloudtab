package resources

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

type UsageEstimateError struct {
	Category string
	Err      error
}

func (e *UsageEstimateError) Error() string { return e.Err.Error() }
func (e *UsageEstimateError) Unwrap() error { return e.Err }

func usageEstimateError(category, format string, args ...interface{}) error {
	return &UsageEstimateError{Category: category, Err: fmt.Errorf(format, args...)}
}

type usageRateMapper struct {
	resourceName string
	allowed      map[string]string
}

func (m usageRateMapper) Extract(parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{}, fmt.Errorf("%s requires versioned usage with supplied rates", m.resourceName)
}

func (m usageRateMapper) Parse(pricing.PriceRequest, []byte) ([]output.CostComponent, error) {
	return nil, fmt.Errorf("%s requires versioned usage with supplied rates", m.resourceName)
}

func (m usageRateMapper) EstimateUsage(_ parser.PlannedResource, usage parser.UsageResource) ([]output.CostComponent, error) {
	if len(usage.Items) == 0 {
		return nil, usageEstimateError("usage_error", "%s requires at least one usage item", m.resourceName)
	}
	for id := range usage.Items {
		if _, ok := m.allowed[id]; !ok {
			return nil, usageEstimateError("usage_error", "unsupported usage item %q for %s", id, m.resourceName)
		}
	}
	for id := range m.allowed {
		if _, ok := usage.Items[id]; !ok {
			return nil, usageEstimateError("usage_error", "usage item %q for %s is required; supply an explicit zero when it does not apply", id, m.resourceName)
		}
	}
	ids := make([]string, 0, len(usage.Items))
	for id := range usage.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	components := make([]output.CostComponent, 0, len(ids))
	runningTotal := 0.0
	for _, id := range ids {
		item := usage.Items[id]
		unit, ok := m.allowed[id]
		if !ok {
			return nil, usageEstimateError("usage_error", "unsupported usage item %q for %s", id, m.resourceName)
		}
		if item.Unit != unit {
			return nil, usageEstimateError("usage_error", "usage item %q for %s requires unit %q, got %q", id, m.resourceName, unit, item.Unit)
		}
		if !finiteUsageNumber(item.Quantity) || item.Quantity < 0 {
			return nil, usageEstimateError("usage_error", "usage item %q for %s requires a nonnegative finite quantity", id, m.resourceName)
		}
		if item.Pricing != "supplied" {
			return nil, usageEstimateError("rate_required", "usage item %q for %s requires pricing supplied", id, m.resourceName)
		}
		if !finiteUsageNumber(item.Rate.Amount) || item.Rate.Amount < 0 || !finiteUsageNumber(item.Rate.Per) || item.Rate.Per <= 0 || strings.TrimSpace(item.Rate.Currency) == "" {
			return nil, usageEstimateError("rate_required", "usage item %q for %s requires a valid supplied rate", id, m.resourceName)
		}
		monthly := item.Quantity * item.Rate.Amount / item.Rate.Per
		if !finiteUsageNumber(monthly) {
			return nil, usageEstimateError("rate_required", "usage item %q for %s produces a non-finite monthly cost", id, m.resourceName)
		}
		runningTotal += monthly
		if !finiteUsageNumber(runningTotal) {
			return nil, usageEstimateError("rate_required", "%s usage items produce a non-finite monthly aggregate", m.resourceName)
		}
		components = append(components, output.CostComponent{
			Name:        id,
			Unit:        "MONTH",
			MonthlyCost: monthly,
			Currency:    item.Rate.Currency,
			Usage: &output.UsageEvidence{
				Quantity: item.Quantity,
				Unit:     item.Unit,
			},
			Rate: &output.RateEvidence{
				Amount:   item.Rate.Amount,
				Per:      item.Rate.Per,
				Currency: item.Rate.Currency,
			},
			Provenance: &output.ProvenanceEvidence{
				Kind: "user_rate_estimate",
				Source: output.SourceEvidence{
					Kind:       item.Rate.Source.Kind,
					Reference:  item.Rate.Source.Reference,
					AsOf:       item.Rate.Source.AsOf,
					Confidence: item.Rate.Source.Confidence,
				},
			},
		})
	}
	return components, nil
}

func finiteUsageNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

type COSBucket struct{ usageRateMapper }
type CDNDomain struct{ usageRateMapper }
type CFSFileSystem struct{ usageRateMapper }
type SCFFunction struct{ usageRateMapper }
type AWSS3Bucket struct{ usageRateMapper }
type AWSEFSFileSystem struct{ usageRateMapper }
type AWSEIP struct{ usageRateMapper }

func newCOSBucket() *COSBucket {
	return &COSBucket{usageRateMapper: newUsageRateMapper("tencentcloud_cos_bucket", map[string]string{
		"standard_storage": "GB-month",
		"get_requests":     "request",
		"put_requests":     "request",
		"internet_egress":  "GB",
	})}
}

func newCDNDomain() *CDNDomain {
	return &CDNDomain{usageRateMapper: newUsageRateMapper("tencentcloud_cdn_domain", map[string]string{
		"traffic":        "GB",
		"https_requests": "request",
	})}
}

func newCFSFileSystem() *CFSFileSystem {
	return &CFSFileSystem{usageRateMapper: newUsageRateMapper("tencentcloud_cfs_file_system", map[string]string{
		"standard_storage": "GB-month",
	})}
}

func newSCFFunction() *SCFFunction {
	return &SCFFunction{usageRateMapper: newUsageRateMapper("tencentcloud_scf_function", map[string]string{
		"compute":         "GB-second",
		"invocations":     "request",
		"internet_egress": "GB",
	})}
}

func newAWSS3Bucket() *AWSS3Bucket {
	return &AWSS3Bucket{usageRateMapper: newUsageRateMapper("aws_s3_bucket", map[string]string{
		"standard_storage": "GB-month",
		"get_requests":     "request",
		"put_requests":     "request",
		"internet_egress":  "GB",
	})}
}

func newAWSEFSFileSystem() *AWSEFSFileSystem {
	return &AWSEFSFileSystem{usageRateMapper: newUsageRateMapper("aws_efs_file_system", map[string]string{
		"standard_storage": "GB-month",
	})}
}

func newAWSEIP() *AWSEIP {
	return &AWSEIP{usageRateMapper: newUsageRateMapper("aws_eip", map[string]string{
		"address_hours": "address-hour",
	})}
}

func newUsageRateMapper(resourceName string, allowed map[string]string) usageRateMapper {
	return usageRateMapper{resourceName: resourceName, allowed: allowed}
}
