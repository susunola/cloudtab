package resources

// AWS MemoryDB, Amazon MQ and Amazon MSK instance pricing.
//
// These three services share a quirk: their Price List products do NOT expose a
// clean `instanceType` filter attribute — the node/broker type is embedded in
// the `usagetype` string (e.g. MSK "USE2-Kafka.m5.large", MQ
// "...InstanceUsage:mq.m5.large"). So instead of filtering on instanceType we
// filter on the attributes that DO exist (location, engine, deploymentOption,
// productFamily) and then pin the right SKU by matching a usagetype substring
// that contains the requested instance type. This mirrors how the ELB mapper
// disambiguates its multiple SKUs via parseAWSPriceListMatching.
//
//   - aws_memorydb_cluster        -> ServiceCode "MemoryDB"
//   - aws_mq_broker               -> ServiceCode "AmazonMQ"
//   - aws_msk_cluster             -> ServiceCode "AmazonMSK"
//
// All prices are per node/broker-hour, scaled by the node/broker count and
// converted to a monthly figure (x 730). Storage, data transfer and other
// usage-driven lines are excluded (they cannot be known from a plan).

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// awsUsageTypeKey stashes, in Params, the usagetype substring Parse should match
// on, plus the node count and a display label. Kept in Params so it survives
// into Parse without changing the backend contract (the backend only reads
// Params["Filters"]/["MaxResults"]).
const (
	awsMatchKey = "UsageTypeContains"
	awsLabelKey = "DisplayLabel"
)

func awsStashMatch(req *pricing.PriceRequest, contains, label string, count int64) {
	req.Params[awsMatchKey] = contains
	req.Params[awsLabelKey] = label
	req.Params["Quantity"] = count
}

func awsMatchValue(req pricing.PriceRequest, key string) string {
	if req.Params == nil {
		return ""
	}
	if v, ok := req.Params[key].(string); ok {
		return v
	}
	return ""
}

// parseUsageMatched pins the SKU whose usagetype contains the stashed substring,
// scales the hourly rate by the stashed node count, and labels the component.
func parseUsageMatched(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	price, err := parseAWSPriceListMatching(raw, awsMatchValue(req, awsMatchKey))
	if err != nil {
		return nil, err
	}
	count := awsQuantity(req)
	if count <= 0 {
		count = 1
	}
	hourly := price.USD * float64(count)
	label := awsMatchValue(req, awsLabelKey)
	if count > 1 {
		label = fmt.Sprintf("%s x%d", label, count)
	}
	return []output.CostComponent{{
		Name:        label,
		Unit:        "HOUR",
		HourlyCost:  hourly,
		MonthlyCost: awsHourlyToMonthly(hourly),
		Currency:    awsCurrency,
	}}, nil
}

// --- MemoryDB ---------------------------------------------------------------

// AWSMemoryDBCluster handles `aws_memorydb_cluster`. Total nodes =
// num_shards * (1 + num_replicas_per_shard).
type AWSMemoryDBCluster struct{}

func (AWSMemoryDBCluster) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	nodeType := getStr(r.After, "node_type")
	if nodeType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_memorydb_cluster: missing node_type")
	}
	shards := getInt(r.After, "num_shards")
	if shards <= 0 {
		shards = 1 // Terraform default
	}
	replicas := getInt(r.After, "num_replicas_per_shard")
	if replicas < 0 {
		replicas = 0
	}
	nodes := shards * (1 + replicas)
	req := awsPriceRequest("MemoryDB", r.Region,
		awsFilter("location", awsLocation(r.Region)),
	)
	awsStashMatch(&req, nodeType, fmt.Sprintf("MemoryDB %s", nodeType), nodes)
	return req, nil
}

func (AWSMemoryDBCluster) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return parseUsageMatched(req, raw)
}

// --- Amazon MQ --------------------------------------------------------------

// AWSMQBroker handles `aws_mq_broker`.
type AWSMQBroker struct{}

func (AWSMQBroker) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	hostType := getStr(r.After, "host_instance_type")
	if hostType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_mq_broker: missing host_instance_type")
	}
	engine := awsMQEngine(getStr(r.After, "engine_type"))
	if engine == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_mq_broker: unsupported engine_type %q", getStr(r.After, "engine_type"))
	}
	deployment := "Single-AZ"
	mode := getStr(r.After, "deployment_mode")
	if mode == "ACTIVE_STANDBY_MULTI_AZ" || mode == "CLUSTER_MULTI_AZ" {
		deployment = "Multi-AZ"
	}
	req := awsPriceRequest("AmazonMQ", r.Region,
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("brokerEngine", engine),
		awsFilter("deploymentOption", deployment),
		awsFilter("productFamily", "Broker Instances"),
	)
	// The instance type is embedded in usagetype as "...Usage:<host_instance_type>".
	awsStashMatch(&req, hostType, fmt.Sprintf("MQ %s %s (%s)", engine, hostType, deployment), 1)
	return req, nil
}

func (AWSMQBroker) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return parseUsageMatched(req, raw)
}

// awsMQEngine maps Terraform engine_type to the Price List brokerEngine value.
func awsMQEngine(engineType string) string {
	switch engineType {
	case "ActiveMQ":
		return "ActiveMQ"
	case "RabbitMQ":
		return "RabbitMQ"
	default:
		return ""
	}
}

// --- Amazon MSK -------------------------------------------------------------

// AWSMSKCluster handles `aws_msk_cluster` (provisioned).
type AWSMSKCluster struct{}

func (AWSMSKCluster) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	bng := getNestedMap(r.After, "broker_node_group_info")
	if bng == nil {
		return pricing.PriceRequest{}, fmt.Errorf("aws_msk_cluster: missing broker_node_group_info")
	}
	instanceType := getStr(bng, "instance_type")
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_msk_cluster: missing broker_node_group_info.instance_type")
	}
	brokers := getInt(r.After, "number_of_broker_nodes")
	if brokers <= 0 {
		brokers = 1
	}
	req := awsPriceRequest("AmazonMSK", r.Region,
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("productFamily", "Managed Streaming for Apache Kafka (MSK)"),
	)
	// MSK broker usagetype looks like "USE2-Kafka.m5.large"; the instance_type
	// (e.g. "kafka.m5.large") shares the "m5.large" tail, so match on that.
	awsStashMatch(&req, mskUsageFragment(instanceType), fmt.Sprintf("MSK %s", instanceType), brokers)
	return req, nil
}

func (AWSMSKCluster) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return parseUsageMatched(req, raw)
}

// mskUsageFragment turns a Terraform MSK instance_type ("kafka.m5.large") into
// the fragment that appears in the Price List usagetype ("Kafka.m5.large").
// Terraform uses a lowercase "kafka." prefix; the usagetype uses "Kafka.". We
// match on the shared instance tail ("m5.large") to be robust to either.
func mskUsageFragment(instanceType string) string {
	const prefix = "kafka."
	if len(instanceType) > len(prefix) && instanceType[:len(prefix)] == prefix {
		return instanceType[len(prefix):]
	}
	return instanceType
}
