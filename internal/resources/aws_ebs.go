package resources

// AWS EBS volume pricing — Terraform `aws_ebs_volume`.
//
// ServiceCode: AmazonEC2 (EBS lives under EC2). Storage is priced per GB-month,
// so the monthly cost is simply pricePerGBMonth * size. We do NOT bill IOPS or
// throughput provisioning here (gp3/io2 extras) — those depend on provisioned
// values that, while present in the plan, are a smaller second-order cost; the
// component name notes the volume type so the base storage figure is clear.
//
// The Price List returns several EBS SKUs for one filter set (per-GB storage,
// per-IOPS, per-throughput). Parse pins the storage line by matching the
// product "usagetype" on "EBS:VolumeUsage" so a gp3/io2 volume is never priced
// off the wrong IOPS/throughput dimension.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSEBSVolume handles `aws_ebs_volume`.
type AWSEBSVolume struct{}

func (AWSEBSVolume) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	volType := getStr(r.After, "type")
	if volType == "" {
		volType = "gp2" // Terraform's default when type is unset
	}
	size := getInt(r.After, "size")
	if size <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("aws_ebs_volume: missing or zero size")
	}
	loc, err := awsLocation(r.Region)
	if err != nil {
		return pricing.PriceRequest{}, err
	}
	req := awsPriceRequest("AmazonEC2", r.Region,
		awsFilter("volumeApiName", volType),
		awsFilter("location", loc),
		awsFilter("productFamily", "Storage"),
	)
	// Stash the provisioned size so Parse can multiply the per-GB-month unit
	// price by it. The AWS backend only reads Params["Filters"]/["MaxResults"],
	// so this extra key is ignored by the query and does not affect the cache
	// key beyond correctly distinguishing volumes of different sizes.
	req.Params["Quantity"] = size
	return req, nil
}

func (AWSEBSVolume) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// Pin the storage line (usagetype "...EBS:VolumeUsage..."); IOPS/throughput
	// dimensions must be excluded so gp3/io2 volumes price off storage only.
	price, err := parseAWSPriceListMatching(raw, "EBS:VolumeUsage")
	if err != nil {
		return nil, err
	}
	volType := filterValue(req, "volumeApiName")

	// EBS storage is quoted per GB-month, so multiply by the provisioned size
	// that Extract stashed in Params["Quantity"].
	size := awsQuantity(req)
	if size <= 0 {
		size = 1
	}
	monthly := price.USD * float64(size)
	return []output.CostComponent{{
		Name:        fmt.Sprintf("EBS %s (%d GB)", volType, size),
		Unit:        "GB-MONTH",
		HourlyCost:  0,
		MonthlyCost: monthly,
		Currency:    awsCurrency,
	}}, nil
}
