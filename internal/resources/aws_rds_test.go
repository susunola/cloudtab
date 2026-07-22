package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

func TestAWSDBInstanceExtract(t *testing.T) {
	r := parser.PlannedResource{
		Address: "aws_db_instance.db",
		Type:    "aws_db_instance",
		Region:  "us-east-1",
		After: map[string]interface{}{
			"instance_class": "db.t3.micro",
			"engine":         "postgres",
			"multi_az":       true,
		},
	}
	req, err := AWSDBInstance{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Provider != "aws" || req.Product != "AmazonRDS" {
		t.Fatalf("route = %s/%s, want aws/AmazonRDS", req.Provider, req.Product)
	}
	if got := filterValue(req, "instanceType"); got != "db.t3.micro" {
		t.Fatalf("instanceType = %q, want db.t3.micro", got)
	}
	if got := filterValue(req, "databaseEngine"); got != "PostgreSQL" {
		t.Fatalf("databaseEngine = %q, want PostgreSQL", got)
	}
	if got := filterValue(req, "deploymentOption"); got != "Multi-AZ" {
		t.Fatalf("deploymentOption = %q, want Multi-AZ", got)
	}
	if got := filterValue(req, "productFamily"); got != "Database Instance" {
		t.Fatalf("productFamily = %q, want Database Instance", got)
	}
}

func TestAWSDBInstanceExtractSingleAZDefault(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "aws_db_instance",
		Region: "us-east-1",
		After: map[string]interface{}{
			"instance_class": "db.m5.large",
			"engine":         "mysql",
		},
	}
	req, err := AWSDBInstance{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got := filterValue(req, "deploymentOption"); got != "Single-AZ" {
		t.Fatalf("deploymentOption = %q, want Single-AZ default", got)
	}
	if got := filterValue(req, "databaseEngine"); got != "MySQL" {
		t.Fatalf("databaseEngine = %q, want MySQL", got)
	}
}

func TestAWSDBInstanceExtractErrors(t *testing.T) {
	// Missing instance_class.
	if _, err := (AWSDBInstance{}).Extract(parser.PlannedResource{
		Type:  "aws_db_instance",
		After: map[string]interface{}{"engine": "mysql"},
	}); err == nil {
		t.Fatal("expected error for missing instance_class")
	}
	// Unsupported engine.
	if _, err := (AWSDBInstance{}).Extract(parser.PlannedResource{
		Type: "aws_db_instance",
		After: map[string]interface{}{
			"instance_class": "db.t3.micro",
			"engine":         "cassandra",
		},
	}); err == nil {
		t.Fatal("expected error for unsupported engine")
	}
}

func TestAWSDBInstanceParse(t *testing.T) {
	req := awsPriceRequest("AmazonRDS", "us-east-1",
		awsFilter("instanceType", "db.t3.micro"),
		awsFilter("databaseEngine", "PostgreSQL"),
		awsFilter("deploymentOption", "Single-AZ"),
	)
	raw := []byte(`[
		{
			"product": {"attributes": {"instanceType": "db.t3.micro"}},
			"terms": {"OnDemand": {"X": {"priceDimensions": {"X.1": {
				"unit": "Hrs",
				"pricePerUnit": {"USD": "0.017000000"}
			}}}}}
		}
	]`)
	comps, err := AWSDBInstance{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if !almostEqAWS(c.HourlyCost, 0.017) {
		t.Fatalf("HourlyCost = %v, want 0.017", c.HourlyCost)
	}
	if !almostEqAWS(c.MonthlyCost, 0.017*hoursPerMonth) {
		t.Fatalf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.017*hoursPerMonth)
	}
	if c.Currency != "USD" {
		t.Fatalf("Currency = %q, want USD", c.Currency)
	}
	if c.Name != "RDS PostgreSQL db.t3.micro (Single-AZ)" {
		t.Fatalf("Name = %q", c.Name)
	}
}

func TestAWSRDSEngineMapping(t *testing.T) {
	cases := map[string]string{
		"mysql":             "MySQL",
		"postgres":          "PostgreSQL",
		"postgresql":        "PostgreSQL",
		"mariadb":           "MariaDB",
		"oracle-se2":        "Oracle",
		"sqlserver-ee":      "SQL Server",
		"aurora-mysql":      "Aurora MySQL",
		"aurora-postgresql": "Aurora PostgreSQL",
		"nope":              "",
	}
	for in, want := range cases {
		if got := awsRDSEngine(in); got != want {
			t.Errorf("awsRDSEngine(%q) = %q, want %q", in, got, want)
		}
	}
}
