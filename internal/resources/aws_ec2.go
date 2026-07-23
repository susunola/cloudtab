package resources

// AWS EC2 instance pricing — Terraform `aws_instance`.
//
// ServiceCode: AmazonEC2. The on-demand hourly rate for a given instance type
// is uniquely pinned by a handful of attribute filters; without them the Price
// List returns dozens of SKUs (reserved, dedicated, per-OS, per-license). We
// pin the common case: shared tenancy, Linux, no pre-installed software, "Used"
// capacity status (the plain on-demand SKU).

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSInstance handles `aws_instance`.
type AWSInstance struct{}

func (AWSInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := getStr(r.After, "instance_type")
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_instance: missing instance_type")
	}
	tenancy := awsTenancy(getStr(r.After, "tenancy"))

	return awsPriceRequest("AmazonEC2", r.Region,
		awsFilter("instanceType", instanceType),
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("tenancy", tenancy),
		awsFilter("operatingSystem", "Linux"),
		awsFilter("preInstalledSw", "NA"),
		awsFilter("capacitystatus", "Used"),
		// marketoption Convertible/OnDemand share this; OnDemand term is what we
		// read in Parse, so no extra filter is needed here.
	), nil
}

func (AWSInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	instanceType := filterValue(req, "instanceType")
	return awsSimpleCost(fmt.Sprintf("EC2 %s (Linux, on-demand)", instanceType), raw)
}

// awsTenancy maps the Terraform tenancy value to the Price List "tenancy"
// attribute. Terraform uses "default"|"dedicated"|"host"; the Price List uses
// "Shared"|"Dedicated"|"Host".
func awsTenancy(tf string) string {
	switch tf {
	case "dedicated":
		return "Dedicated"
	case "host":
		return "Host"
	default:
		return "Shared"
	}
}
