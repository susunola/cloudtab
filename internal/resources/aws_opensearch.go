package resources

// AWS OpenSearch / Elasticsearch pricing — Terraform `aws_opensearch_domain`
// and the legacy `aws_elasticsearch_domain`.
//
// ServiceCode: AmazonES (OpenSearch Service still bills under the AmazonES
// offer code). A domain's cost is per data-node-hour, pinned by instanceType
// and location, times the number of data nodes. The instance_type in the plan
// already carries the Price List suffix (".search" for OpenSearch generations,
// ".elasticsearch" for legacy ES), so we pass it through verbatim. Dedicated
// master nodes / EBS storage / UltraWarm are separate lines we do not price
// here — the data-node compute is the dominant, deterministic figure.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSOpenSearchDomain handles `aws_opensearch_domain` and
// `aws_elasticsearch_domain` (identical schema for our purposes).
type AWSOpenSearchDomain struct{}

func (AWSOpenSearchDomain) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	cc := getNestedMap(r.After, "cluster_config")
	if cc == nil {
		return pricing.PriceRequest{}, fmt.Errorf("aws_opensearch_domain: missing cluster_config")
	}
	instanceType := getStr(cc, "instance_type")
	if instanceType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_opensearch_domain: missing cluster_config.instance_type")
	}
	count := getInt(cc, "instance_count")
	if count <= 0 {
		count = 1 // Terraform defaults instance_count to 1
	}
	req := awsPriceRequest("AmazonES", r.Region,
		awsFilter("instanceType", instanceType),
		awsFilter("location", awsLocation(r.Region)),
	)
	req.Params["Quantity"] = count
	return req, nil
}

func (AWSOpenSearchDomain) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	price, err := parseAWSPriceList(raw)
	if err != nil {
		return nil, err
	}
	instanceType := filterValue(req, "instanceType")
	count := awsQuantity(req)
	if count <= 0 {
		count = 1
	}
	hourly := price.USD * float64(count)
	name := fmt.Sprintf("OpenSearch %s", instanceType)
	if count > 1 {
		name = fmt.Sprintf("OpenSearch %s x%d", instanceType, count)
	}
	return []output.CostComponent{{
		Name:        name,
		Unit:        "HOUR",
		HourlyCost:  hourly,
		MonthlyCost: awsHourlyToMonthly(hourly),
		Currency:    awsCurrency,
	}}, nil
}
