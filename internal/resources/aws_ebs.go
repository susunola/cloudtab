package resources

// AWS EBS volume pricing — Terraform `aws_ebs_volume`.
//
// ServiceCode: AmazonEC2 (EBS lives under EC2). Storage is priced per GB-month,
// so the monthly cost is simply pricePerGBMonth * size. We do NOT bill IOPS or
// throughput provisioning here (gp3/io2 extras) — those depend on provisioned
// values that, while present in the plan, are a smaller second-order cost; the
// component name notes the volume type so the base storage figure is clear.

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
	req := awsPriceRequest("AmazonEC2", r.Region,
		awsFilter("volumeApiName", volType),
		awsFilter("location", awsLocation(r.Region)),
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
	price, err := parseAWSPriceList(raw)
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
