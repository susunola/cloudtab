package resources

// AWS Aurora / RDS-cluster instance pricing — Terraform
// `aws_rds_cluster_instance`.
//
// An aws_rds_cluster (Aurora) itself has no per-hour instance cost; the cost
// lives on each aws_rds_cluster_instance, which is priced exactly like an
// aws_db_instance: AmazonRDS, productFamily "Database Instance", pinned by the
// instance class, database engine and location. Aurora is always Single-AZ in
// the Price List sense (HA comes from replicas as separate instances, each its
// own aws_rds_cluster_instance), so we do not vary deploymentOption here.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSRDSClusterInstance handles `aws_rds_cluster_instance` (Aurora members).
type AWSRDSClusterInstance struct{}

func (AWSRDSClusterInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := getStr(r.After, "instance_class")
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_rds_cluster_instance: missing instance_class")
	}
	// The engine lives on the parent aws_rds_cluster; aws_rds_cluster_instance
	// also accepts an `engine` attribute (aurora-mysql / aurora-postgresql).
	// Default to Aurora MySQL when unset, which is the common case.
	engine := getStr(r.After, "engine")
	dbEngine := awsRDSEngine(engine)
	if dbEngine == "" {
		dbEngine = "Aurora MySQL"
	}
	return awsPriceRequest("AmazonRDS", r.Region,
		awsFilter("instanceType", instanceType),
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("databaseEngine", dbEngine),
		awsFilter("deploymentOption", "Single-AZ"),
		awsFilter("productFamily", "Database Instance"),
	), nil
}

func (AWSRDSClusterInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	instanceType := filterValue(req, "instanceType")
	engine := filterValue(req, "databaseEngine")
	return awsSimpleCost(fmt.Sprintf("Aurora %s %s", engine, instanceType), raw)
}
