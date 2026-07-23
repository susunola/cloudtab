package resources

// AWS RDS database instance pricing — Terraform `aws_db_instance`.
//
// ServiceCode: AmazonRDS, productFamily "Database Instance". The on-demand
// hourly rate for a DB instance is pinned by instanceType (the db.* class),
// databaseEngine, deploymentOption (Single-AZ / Multi-AZ) and location. We do
// NOT price allocated storage, IOPS, or backup here — those are separate
// productFamily lines ("Database Storage" etc.); the instance compute cost is
// the dominant, deterministic figure and what a plan-time estimate needs.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSDBInstance handles `aws_db_instance`.
type AWSDBInstance struct{}

func (AWSDBInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	instanceType := getStr(r.After, "instance_class")
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_db_instance: missing instance_class")
	}
	engine := getStr(r.After, "engine")
	dbEngine := awsRDSEngine(engine)
	if dbEngine == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_db_instance: unsupported engine %q", engine)
	}
	deployment := "Single-AZ"
	if getBool(r.After, "multi_az") {
		deployment = "Multi-AZ"
	}
	return awsPriceRequest("AmazonRDS", r.Region,
		awsFilter("instanceType", instanceType),
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("databaseEngine", dbEngine),
		awsFilter("deploymentOption", deployment),
		awsFilter("productFamily", "Database Instance"),
	), nil
}

func (AWSDBInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	instanceType := filterValue(req, "instanceType")
	engine := filterValue(req, "databaseEngine")
	deployment := filterValue(req, "deploymentOption")
	return awsSimpleCost(fmt.Sprintf("RDS %s %s (%s)", engine, instanceType, deployment), raw)
}

// awsRDSEngine maps the Terraform `engine` value (lowercase engine IDs like
// "mysql", "postgres", "aurora-mysql") to the Price List "databaseEngine"
// attribute value ("MySQL", "PostgreSQL", "Aurora MySQL", ...). Returns "" for
// engines we cannot price (so Extract can report a clear error rather than
// silently querying the wrong SKU). Version suffixes (e.g. "sqlserver-ee") are
// normalised to their family.
func awsRDSEngine(engine string) string {
	switch engine {
	case "mysql":
		return "MySQL"
	case "postgres", "postgresql":
		return "PostgreSQL"
	case "mariadb":
		return "MariaDB"
	case "oracle-se", "oracle-se1", "oracle-se2", "oracle-ee", "oracle-ee-cdb", "oracle-se2-cdb":
		return "Oracle"
	case "sqlserver-ee", "sqlserver-se", "sqlserver-ex", "sqlserver-web":
		return "SQL Server"
	case "aurora-mysql", "aurora":
		return "Aurora MySQL"
	case "aurora-postgresql":
		return "Aurora PostgreSQL"
	default:
		return ""
	}
}
