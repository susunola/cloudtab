package resources

// AWS fixed-hourly-fee resources — Terraform `aws_eks_cluster` (control plane)
// and `aws_nat_gateway`.
//
// Both have a deterministic fixed hourly charge plus usage-driven charges that
// a plan cannot know:
//   - EKS: the control plane is a flat per-cluster hourly fee; worker nodes
//     (EC2 / Fargate) are separate resources with their own cost.
//   - NAT gateway: a fixed hourly presence fee PLUS a per-GB data-processing
//     fee that depends on live traffic. We price ONLY the fixed hourly line and
//     label it so the excluded data-processing cost is explicit (same approach
//     as the ELB mapper).

import (
	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSEKSCluster handles `aws_eks_cluster` (control plane only).
type AWSEKSCluster struct{}

func (AWSEKSCluster) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return awsPriceRequest("AmazonEKS", r.Region,
		awsFilter("location", awsLocation(r.Region)),
	), nil
}

func (AWSEKSCluster) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// The EKS cluster-hour SKU has usagetype "...AmazonEKS-Hours:perCluster".
	price, err := parseAWSPriceListMatching(raw, "AmazonEKS-Hours")
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "EKS control plane (per cluster)",
		Unit:        "HOUR",
		HourlyCost:  price.USD,
		MonthlyCost: awsHourlyToMonthly(price.USD),
		Currency:    awsCurrency,
	}}, nil
}

// AWSNATGateway handles `aws_nat_gateway`.
type AWSNATGateway struct{}

func (AWSNATGateway) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	// NAT gateway lives under the EC2 service code.
	return awsPriceRequest("AmazonEC2", r.Region,
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("productFamily", "NAT Gateway"),
	), nil
}

func (AWSNATGateway) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// Pin the fixed hourly presence fee (usagetype "...NatGateway-Hours"),
	// excluding the per-GB "NatGateway-Bytes" data-processing SKU.
	price, err := parseAWSPriceListMatching(raw, "NatGateway-Hours")
	if err != nil {
		return nil, err
	}
	return []output.CostComponent{{
		Name:        "NAT gateway hourly (base, excl. data processing)",
		Unit:        "HOUR",
		HourlyCost:  price.USD,
		MonthlyCost: awsHourlyToMonthly(price.USD),
		Currency:    awsCurrency,
	}}, nil
}
