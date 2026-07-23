package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

// hrsDoc builds a single-product price-list JSON array with one OnDemand USD
// hourly rate and an optional usagetype attribute.
func hrsDoc(usd, usagetype string) []byte {
	ut := ""
	if usagetype != "" {
		ut = `"usagetype": "` + usagetype + `",`
	}
	return []byte(`[
		{"product": {"attributes": {` + ut + `"x": "y"}},
		 "terms": {"OnDemand": {"T": {"priceDimensions": {"D": {
			"unit": "Hrs", "pricePerUnit": {"USD": "` + usd + `"}}}}}}}
	]`)
}

// --- Aurora (aws_rds_cluster_instance) --------------------------------------

func TestAWSRDSClusterInstance(t *testing.T) {
	r := parser.PlannedResource{
		Type: "aws_rds_cluster_instance", Region: "us-east-1",
		After: map[string]interface{}{"instance_class": "db.r6g.large", "engine": "aurora-postgresql"},
	}
	req, err := AWSRDSClusterInstance{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if req.Product != "AmazonRDS" || filterValue(req, "databaseEngine") != "Aurora PostgreSQL" {
		t.Fatalf("route/engine wrong: %s / %s", req.Product, filterValue(req, "databaseEngine"))
	}
	if filterValue(req, "deploymentOption") != "Single-AZ" {
		t.Fatalf("aurora should be Single-AZ")
	}
	comps, err := AWSRDSClusterInstance{}.Parse(req, hrsDoc("0.29", ""))
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if len(comps) != 1 || !almostEqAWS(comps[0].HourlyCost, 0.29) {
		t.Fatalf("bad component %+v", comps)
	}
	if comps[0].Name != "Aurora Aurora PostgreSQL db.r6g.large" {
		t.Fatalf("name = %q", comps[0].Name)
	}
}

func TestAWSRDSClusterInstanceDefaultsAuroraMySQL(t *testing.T) {
	req, err := AWSRDSClusterInstance{}.Extract(parser.PlannedResource{
		Type: "aws_rds_cluster_instance", Region: "eu-west-1",
		After: map[string]interface{}{"instance_class": "db.t3.medium"},
	})
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if got := filterValue(req, "databaseEngine"); got != "Aurora MySQL" {
		t.Fatalf("default engine = %q, want Aurora MySQL", got)
	}
}

func TestAWSRDSClusterInstanceMissingClass(t *testing.T) {
	if _, err := (AWSRDSClusterInstance{}).Extract(parser.PlannedResource{
		Type: "aws_rds_cluster_instance", After: map[string]interface{}{},
	}); err == nil {
		t.Fatal("expected missing instance_class error")
	}
}

// --- Redshift ----------------------------------------------------------------

func TestAWSRedshiftMultiNode(t *testing.T) {
	r := parser.PlannedResource{
		Type: "aws_redshift_cluster", Region: "us-east-1",
		After: map[string]interface{}{"node_type": "ra3.xlplus", "number_of_nodes": float64(3)},
	}
	req, err := AWSRedshiftCluster{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if req.Product != "AmazonRedshift" || filterValue(req, "instanceType") != "ra3.xlplus" {
		t.Fatalf("route/type wrong")
	}
	comps, err := AWSRedshiftCluster{}.Parse(req, hrsDoc("1.086", ""))
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 1.086*3) {
		t.Fatalf("hourly = %v, want %v", comps[0].HourlyCost, 1.086*3)
	}
	if comps[0].Name != "Redshift ra3.xlplus x3" {
		t.Fatalf("name = %q", comps[0].Name)
	}
}

func TestAWSRedshiftSingleNodeDefault(t *testing.T) {
	req, err := AWSRedshiftCluster{}.Extract(parser.PlannedResource{
		Type: "aws_redshift_cluster", Region: "us-east-1",
		After: map[string]interface{}{"node_type": "dc2.large"},
	})
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if awsQuantity(req) != 1 {
		t.Fatalf("default nodes = %d, want 1", awsQuantity(req))
	}
}

func TestAWSRedshiftMissingNodeType(t *testing.T) {
	if _, err := (AWSRedshiftCluster{}).Extract(parser.PlannedResource{
		Type: "aws_redshift_cluster", After: map[string]interface{}{},
	}); err == nil {
		t.Fatal("expected missing node_type error")
	}
}

// --- OpenSearch --------------------------------------------------------------

func TestAWSOpenSearchNestedConfig(t *testing.T) {
	r := parser.PlannedResource{
		Type: "aws_opensearch_domain", Region: "us-east-1",
		After: map[string]interface{}{
			"cluster_config": []interface{}{map[string]interface{}{
				"instance_type": "m5.large.search", "instance_count": float64(2),
			}},
		},
	}
	req, err := AWSOpenSearchDomain{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if req.Product != "AmazonES" || filterValue(req, "instanceType") != "m5.large.search" {
		t.Fatalf("route/type wrong: %s / %s", req.Product, filterValue(req, "instanceType"))
	}
	comps, err := AWSOpenSearchDomain{}.Parse(req, hrsDoc("0.167", ""))
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.167*2) {
		t.Fatalf("hourly = %v, want %v", comps[0].HourlyCost, 0.167*2)
	}
	if comps[0].Name != "OpenSearch m5.large.search x2" {
		t.Fatalf("name = %q", comps[0].Name)
	}
}

func TestAWSOpenSearchMapAsObject(t *testing.T) {
	// cluster_config as a bare map (not a list) must also work.
	req, err := AWSOpenSearchDomain{}.Extract(parser.PlannedResource{
		Type: "aws_opensearch_domain", Region: "us-east-1",
		After: map[string]interface{}{
			"cluster_config": map[string]interface{}{"instance_type": "t3.small.search"},
		},
	})
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if filterValue(req, "instanceType") != "t3.small.search" {
		t.Fatalf("instanceType wrong")
	}
	if awsQuantity(req) != 1 {
		t.Fatalf("default count = %d, want 1", awsQuantity(req))
	}
}

func TestAWSOpenSearchMissingConfig(t *testing.T) {
	if _, err := (AWSOpenSearchDomain{}).Extract(parser.PlannedResource{
		Type: "aws_opensearch_domain", After: map[string]interface{}{},
	}); err == nil {
		t.Fatal("expected missing cluster_config error")
	}
}

// --- DocumentDB / Neptune ----------------------------------------------------

func TestAWSDocDBAndNeptune(t *testing.T) {
	dreq, err := AWSDocDBInstance{}.Extract(parser.PlannedResource{
		Type: "aws_docdb_cluster_instance", Region: "us-east-1",
		After: map[string]interface{}{"instance_class": "db.r5.large"},
	})
	if err != nil {
		t.Fatalf("docdb extract err %v", err)
	}
	if dreq.Product != "AmazonDocDB" {
		t.Fatalf("docdb service = %s", dreq.Product)
	}
	dcomps, _ := AWSDocDBInstance{}.Parse(dreq, hrsDoc("0.277", ""))
	if dcomps[0].Name != "DocumentDB db.r5.large" {
		t.Fatalf("docdb name = %q", dcomps[0].Name)
	}

	nreq, err := AWSNeptuneInstance{}.Extract(parser.PlannedResource{
		Type: "aws_neptune_cluster_instance", Region: "us-east-1",
		After: map[string]interface{}{"instance_class": "db.r5.xlarge"},
	})
	if err != nil {
		t.Fatalf("neptune extract err %v", err)
	}
	if nreq.Product != "AmazonNeptune" {
		t.Fatalf("neptune service = %s", nreq.Product)
	}
	ncomps, _ := AWSNeptuneInstance{}.Parse(nreq, hrsDoc("0.348", ""))
	if !almostEqAWS(ncomps[0].HourlyCost, 0.348) || ncomps[0].Name != "Neptune db.r5.xlarge" {
		t.Fatalf("neptune component wrong %+v", ncomps[0])
	}
}

func TestAWSDocDBMissingClass(t *testing.T) {
	if _, err := (AWSDocDBInstance{}).Extract(parser.PlannedResource{
		Type: "aws_docdb_cluster_instance", After: map[string]interface{}{},
	}); err == nil {
		t.Fatal("expected missing instance_class error")
	}
}

// --- MemoryDB (usagetype matching + node math) -------------------------------

func TestAWSMemoryDBNodeMath(t *testing.T) {
	// 2 shards x (1 primary + 1 replica) = 4 nodes.
	r := parser.PlannedResource{
		Type: "aws_memorydb_cluster", Region: "us-east-1",
		After: map[string]interface{}{
			"node_type": "db.r6g.large", "num_shards": float64(2), "num_replicas_per_shard": float64(1),
		},
	}
	req, err := AWSMemoryDBCluster{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if req.Product != "MemoryDB" || awsQuantity(req) != 4 {
		t.Fatalf("service/nodes wrong: %s / %d", req.Product, awsQuantity(req))
	}
	// usagetype must contain the node type to be matched.
	comps, err := AWSMemoryDBCluster{}.Parse(req, hrsDoc("0.212", "USE1-NodeUsage:db.r6g.large"))
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.212*4) {
		t.Fatalf("hourly = %v, want %v", comps[0].HourlyCost, 0.212*4)
	}
	if comps[0].Name != "MemoryDB db.r6g.large x4" {
		t.Fatalf("name = %q", comps[0].Name)
	}
}

func TestAWSMemoryDBSkipsWrongUsageType(t *testing.T) {
	req, _ := AWSMemoryDBCluster{}.Extract(parser.PlannedResource{
		Type: "aws_memorydb_cluster", Region: "us-east-1",
		After: map[string]interface{}{"node_type": "db.r6g.large"},
	})
	// Only a storage SKU (no matching node usagetype) -> error, never a wrong price.
	if _, err := (AWSMemoryDBCluster{}).Parse(req, hrsDoc("0.01", "USE1-SnapshotStorage")); err == nil {
		t.Fatal("expected no-match error when usagetype does not contain node type")
	}
}

// --- Amazon MQ ---------------------------------------------------------------

func TestAWSMQBroker(t *testing.T) {
	r := parser.PlannedResource{
		Type: "aws_mq_broker", Region: "us-east-1",
		After: map[string]interface{}{
			"host_instance_type": "mq.m5.large", "engine_type": "RabbitMQ",
			"deployment_mode": "ACTIVE_STANDBY_MULTI_AZ",
		},
	}
	req, err := AWSMQBroker{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if req.Product != "AmazonMQ" || filterValue(req, "brokerEngine") != "RabbitMQ" {
		t.Fatalf("route/engine wrong")
	}
	if filterValue(req, "deploymentOption") != "Multi-AZ" {
		t.Fatalf("deployment = %q, want Multi-AZ", filterValue(req, "deploymentOption"))
	}
	comps, err := AWSMQBroker{}.Parse(req, hrsDoc("0.598", "EUW1-RabbitMQ-Multi-InstanceUsage:mq.m5.large"))
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.598) {
		t.Fatalf("hourly = %v", comps[0].HourlyCost)
	}
	if comps[0].Name != "MQ RabbitMQ mq.m5.large (Multi-AZ)" {
		t.Fatalf("name = %q", comps[0].Name)
	}
}

func TestAWSMQUnsupportedEngine(t *testing.T) {
	if _, err := (AWSMQBroker{}).Extract(parser.PlannedResource{
		Type: "aws_mq_broker",
		After: map[string]interface{}{
			"host_instance_type": "mq.m5.large", "engine_type": "Kafka",
		},
	}); err == nil {
		t.Fatal("expected unsupported engine error")
	}
}

// --- Amazon MSK --------------------------------------------------------------

func TestAWSMSKCluster(t *testing.T) {
	r := parser.PlannedResource{
		Type: "aws_msk_cluster", Region: "us-east-1",
		After: map[string]interface{}{
			"number_of_broker_nodes": float64(3),
			"broker_node_group_info": []interface{}{map[string]interface{}{
				"instance_type": "kafka.m5.large",
			}},
		},
	}
	req, err := AWSMSKCluster{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if req.Product != "AmazonMSK" || awsQuantity(req) != 3 {
		t.Fatalf("service/brokers wrong: %s / %d", req.Product, awsQuantity(req))
	}
	// usagetype "USE1-Kafka.m5.large" contains fragment "m5.large".
	comps, err := AWSMSKCluster{}.Parse(req, hrsDoc("0.21", "USE1-Kafka.m5.large"))
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.21*3) {
		t.Fatalf("hourly = %v, want %v", comps[0].HourlyCost, 0.21*3)
	}
	if comps[0].Name != "MSK kafka.m5.large x3" {
		t.Fatalf("name = %q", comps[0].Name)
	}
}

func TestMSKUsageFragment(t *testing.T) {
	if got := mskUsageFragment("kafka.m5.large"); got != "m5.large" {
		t.Fatalf("fragment = %q, want m5.large", got)
	}
	if got := mskUsageFragment("m5.large"); got != "m5.large" {
		t.Fatalf("fragment passthrough = %q", got)
	}
}

// --- DynamoDB (provisioned only) ---------------------------------------------

func TestAWSDynamoDBProvisioned(t *testing.T) {
	r := parser.PlannedResource{
		Type: "aws_dynamodb_table", Region: "us-east-1",
		After: map[string]interface{}{
			"billing_mode": "PROVISIONED", "read_capacity": float64(10), "write_capacity": float64(5),
		},
	}
	req, err := AWSDynamoDBTable{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	raw := []byte(`[
		{"product": {"attributes": {"usagetype": "USE1-ReadCapacityUnit-Hrs"}},
		 "terms": {"OnDemand": {"T": {"priceDimensions": {"D": {
			"unit": "Hrs", "pricePerUnit": {"USD": "0.00013"}}}}}}},
		{"product": {"attributes": {"usagetype": "USE1-WriteCapacityUnit-Hrs"}},
		 "terms": {"OnDemand": {"T": {"priceDimensions": {"D": {
			"unit": "Hrs", "pricePerUnit": {"USD": "0.00065"}}}}}}}
	]`)
	comps, err := AWSDynamoDBTable{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if len(comps) != 2 {
		t.Fatalf("components = %d, want 2 (RCU+WCU)", len(comps))
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.00013*10) {
		t.Fatalf("RCU hourly = %v", comps[0].HourlyCost)
	}
	if !almostEqAWS(comps[1].HourlyCost, 0.00065*5) {
		t.Fatalf("WCU hourly = %v", comps[1].HourlyCost)
	}
}

func TestAWSDynamoDBOnDemandSkipped(t *testing.T) {
	if _, err := (AWSDynamoDBTable{}).Extract(parser.PlannedResource{
		Type: "aws_dynamodb_table", Region: "us-east-1",
		After: map[string]interface{}{"billing_mode": "PAY_PER_REQUEST"},
	}); err == nil {
		t.Fatal("expected on-demand table to be skipped with error")
	}
}

// --- EKS / NAT gateway (fixed fee, usagetype-pinned) -------------------------

func TestAWSEKSControlPlane(t *testing.T) {
	req, err := AWSEKSCluster{}.Extract(parser.PlannedResource{
		Type: "aws_eks_cluster", Region: "us-east-1", After: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	if req.Product != "AmazonEKS" {
		t.Fatalf("service = %s", req.Product)
	}
	comps, err := AWSEKSCluster{}.Parse(req, hrsDoc("0.10", "USE1-AmazonEKS-Hours:perCluster"))
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.10) || comps[0].Name != "EKS control plane (per cluster)" {
		t.Fatalf("eks component wrong %+v", comps[0])
	}
}

func TestAWSNATGatewayExcludesData(t *testing.T) {
	req, err := AWSNATGateway{}.Extract(parser.PlannedResource{
		Type: "aws_nat_gateway", Region: "us-east-1", After: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("Extract err %v", err)
	}
	// Two SKUs: fixed hours + data bytes. Parse must pick the Hours one.
	raw := []byte(`[
		{"product": {"attributes": {"usagetype": "USE1-NatGateway-Bytes"}},
		 "terms": {"OnDemand": {"T": {"priceDimensions": {"D": {
			"unit": "GB", "pricePerUnit": {"USD": "0.045"}}}}}}},
		{"product": {"attributes": {"usagetype": "USE1-NatGateway-Hours"}},
		 "terms": {"OnDemand": {"T": {"priceDimensions": {"D": {
			"unit": "Hrs", "pricePerUnit": {"USD": "0.045"}}}}}}}
	]`)
	comps, err := AWSNATGateway{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse err %v", err)
	}
	if !almostEqAWS(comps[0].HourlyCost, 0.045) {
		t.Fatalf("hourly = %v, want 0.045 (Hours SKU)", comps[0].HourlyCost)
	}
	if comps[0].Name != "NAT gateway hourly (base, excl. data processing)" {
		t.Fatalf("name = %q", comps[0].Name)
	}
}

// --- registry wiring ---------------------------------------------------------

func TestNewAWSMappersRegistered(t *testing.T) {
	reg := DefaultRegistry()
	for _, tfType := range []string{
		"aws_rds_cluster_instance", "aws_redshift_cluster", "aws_opensearch_domain",
		"aws_elasticsearch_domain", "aws_docdb_cluster_instance", "aws_neptune_cluster_instance",
		"aws_memorydb_cluster", "aws_mq_broker", "aws_msk_cluster", "aws_dynamodb_table",
		"aws_eks_cluster", "aws_nat_gateway",
	} {
		if _, ok := reg.Lookup(tfType); !ok {
			t.Errorf("registry missing %s", tfType)
		}
	}
	// S3 / EIP / EFS must remain unregistered by design.
	for _, tfType := range []string{"aws_s3_bucket", "aws_eip", "aws_efs_file_system"} {
		if _, ok := reg.Lookup(tfType); ok {
			t.Errorf("registry should NOT contain %s", tfType)
		}
	}
}
