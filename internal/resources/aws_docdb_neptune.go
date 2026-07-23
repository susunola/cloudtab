package resources

// AWS DocumentDB and Neptune instance pricing — Terraform
// `aws_docdb_cluster_instance` and `aws_neptune_cluster_instance`.
//
// Both are cluster-based engines whose per-hour cost lives on each member
// instance (the cluster resource itself is free). Each instance is priced like
// an RDS instance: productFamily "Database Instance", pinned by instanceType
// (the db.* class) and location, under their own service codes:
//   - AmazonDocDB  for DocumentDB
//   - AmazonNeptune for Neptune
// Storage and I/O are usage-driven and priced separately by AWS; we price the
// deterministic instance compute only.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// awsClusterInstanceRequest builds the shared Database-Instance price request
// used by DocumentDB and Neptune member instances.
func awsClusterInstanceRequest(serviceCode, region, instanceType string) pricing.PriceRequest {
	return awsPriceRequest(serviceCode, region,
		awsFilter("instanceType", instanceType),
		awsFilter("location", awsLocation(region)),
		awsFilter("productFamily", "Database Instance"),
	)
}

// awsClusterInstanceComponent turns the pinned hourly rate into one monthly
// CostComponent labelled with the given engine name.
func awsClusterInstanceComponent(engineLabel string, req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	price, err := parseAWSPriceList(raw)
	if err != nil {
		return nil, err
	}
	instanceType := filterValue(req, "instanceType")
	return []output.CostComponent{{
		Name:        fmt.Sprintf("%s %s", engineLabel, instanceType),
		Unit:        "HOUR",
		HourlyCost:  price.USD,
		MonthlyCost: awsHourlyToMonthly(price.USD),
		Currency:    awsCurrency,
	}}, nil
}

// AWSDocDBInstance handles `aws_docdb_cluster_instance`.
type AWSDocDBInstance struct{}

func (AWSDocDBInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := getStr(r.After, "instance_class")
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_docdb_cluster_instance: missing instance_class")
	}
	return awsClusterInstanceRequest("AmazonDocDB", r.Region, instanceType), nil
}

func (AWSDocDBInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return awsClusterInstanceComponent("DocumentDB", req, raw)
}

// AWSNeptuneInstance handles `aws_neptune_cluster_instance`.
type AWSNeptuneInstance struct{}

func (AWSNeptuneInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := getStr(r.After, "instance_class")
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_neptune_cluster_instance: missing instance_class")
	}
	return awsClusterInstanceRequest("AmazonNeptune", r.Region, instanceType), nil
}

func (AWSNeptuneInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return awsClusterInstanceComponent("Neptune", req, raw)
}
