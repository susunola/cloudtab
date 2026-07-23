package resources

// AWS Redshift cluster pricing — Terraform `aws_redshift_cluster`.
//
// ServiceCode: AmazonRedshift. A provisioned cluster is priced per node-hour by
// node type (instanceType, e.g. dc2.large / ra3.xlplus) and location. A cluster
// runs `number_of_nodes` of them (single-node clusters report 1), so the
// monthly figure is per-node hourly × 730 × node count. We price on-demand
// compute only; RA3 managed storage (RMS) is usage-driven (GB stored) and, like
// S3/EIP, cannot be known from a plan, so it is deliberately excluded.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSRedshiftCluster handles `aws_redshift_cluster`.
type AWSRedshiftCluster struct{}

func (AWSRedshiftCluster) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	nodeType := getStr(r.After, "node_type")
	if nodeType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_redshift_cluster: missing node_type")
	}
	nodes := getInt(r.After, "number_of_nodes")
	if nodes <= 0 {
		nodes = 1 // single-node cluster
	}
	req := awsPriceRequest("AmazonRedshift", r.Region,
		awsFilter("instanceType", nodeType),
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("productFamily", "Compute Instance"),
	)
	req.Params["Quantity"] = nodes
	return req, nil
}

func (AWSRedshiftCluster) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	nodeType := filterValue(req, "instanceType")
	nodes := awsQuantity(req)
	if nodes <= 0 {
		nodes = 1
	}
	name := fmt.Sprintf("Redshift %s", nodeType)
	if nodes > 1 {
		name = fmt.Sprintf("Redshift %s x%d", nodeType, nodes)
	}
	return awsScaledCost(name, float64(nodes), raw)
}
