package resources

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// COSBucket handles `tencentcloud_cos_bucket`.
//
// COS pricing depends on storage class, region, and actual GB stored / requests
// / egress — none of which are present in a Terraform plan. We return a
// zero-cost placeholder line with a descriptive note and route it through the
// StaticMapper path so it never calls the pricing engine. No price is fabricated.
type COSBucket struct{}

func (COSBucket) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{}, fmt.Errorf("COS pricing is static; use Estimate")
}

func (COSBucket) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return nil, fmt.Errorf("COS pricing is static; use Estimate")
}

func (COSBucket) Estimate(r parser.PlannedResource) ([]output.CostComponent, error) {
	return staticUsageNote("COS cost depends on storage class, region and actual GB stored / requests / egress — not in plan; see console"), nil
}

// CDNDomain handles `tencentcloud_cdn_domain`.
//
// Like COS, CDN cost is usage-driven (traffic, requests, acceleration region)
// and not derivable from the plan, so it is reported as a placeholder.
type CDNDomain struct{}

func (CDNDomain) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{}, fmt.Errorf("CDN pricing is static; use Estimate")
}

func (CDNDomain) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return nil, fmt.Errorf("CDN pricing is static; use Estimate")
}

func (CDNDomain) Estimate(r parser.PlannedResource) ([]output.CostComponent, error) {
	return staticUsageNote("CDN cost depends on traffic, requests and acceleration region — not in plan; see console"), nil
}

// CFSFileSystem handles `tencentcloud_cfs_file_system`.
//
// CFS cost depends on storage type, provisioned capacity (GiB) and actual GB
// stored — not present in the plan — so it is reported as a placeholder.
type CFSFileSystem struct{}

func (CFSFileSystem) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{}, fmt.Errorf("CFS pricing is static; use Estimate")
}

func (CFSFileSystem) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return nil, fmt.Errorf("CFS pricing is static; use Estimate")
}

func (CFSFileSystem) Estimate(r parser.PlannedResource) ([]output.CostComponent, error) {
	return staticUsageNote("CFS cost depends on storage type, capacity (GiB) and actual GB stored — not in plan; see console"), nil
}

// SCFFunction handles `tencentcloud_scf_function`.
//
// SCF cost depends on invocations, execution duration and memory (GB-s) — none
// of which are in the plan — so it is reported as a placeholder.
type SCFFunction struct{}

func (SCFFunction) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return pricing.PriceRequest{}, fmt.Errorf("SCF pricing is static; use Estimate")
}

func (SCFFunction) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return nil, fmt.Errorf("SCF pricing is static; use Estimate")
}

func (SCFFunction) Estimate(r parser.PlannedResource) ([]output.CostComponent, error) {
	return staticUsageNote("SCF cost depends on invocations, duration and memory (GB-s) — not in plan; see console"), nil
}

// staticUsageNote builds the zero-cost placeholder CostComponent used by the
// usage-driven Tencent mappers. Cost is always 0 because no price can be derived
// from the plan; the note tells the reader where to look for the real number.
func staticUsageNote(note string) []output.CostComponent {
	return []output.CostComponent{{
		Name:        note,
		Unit:        "MONTH",
		HourlyCost:  0,
		MonthlyCost: 0,
		Currency:    "CNY",
	}}
}
