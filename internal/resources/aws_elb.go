package resources

// AWS Elastic Load Balancing pricing — Terraform `aws_lb` (ALB/NLB/GWLB) and
// `aws_elb` (Classic).
//
// ServiceCode: AWSELB. A load balancer has a deterministic fixed hourly charge
// (the "LoadBalancerUsage" SKU) plus usage-driven capacity charges (LCU / GLCU
// / NLCU / data processing) that depend on live traffic and cannot be known
// from a plan. We price ONLY the fixed hourly base — the component name makes
// clear that LCU/data-processing usage is excluded.
//
// The productFamily distinguishes the four types:
//   - Load Balancer             (Classic, aws_elb)
//   - Load Balancer-Application (ALB, aws_lb type=application)
//   - Load Balancer-Network     (NLB, aws_lb type=network)
//   - Load Balancer-Gateway     (GWLB, aws_lb type=gateway)

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSLB handles `aws_lb` (Application / Network / Gateway load balancers).
type AWSLB struct{}

func (AWSLB) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	lbType := getStr(r.After, "load_balancer_type")
	if lbType == "" {
		lbType = "application" // Terraform's default for aws_lb
	}
	family := awsLBFamily(lbType)
	if family == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_lb: unsupported load_balancer_type %q", lbType)
	}
	return awsELBRequest(r.Region, family), nil
}

func (AWSLB) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return parseELB(req, raw)
}

// AWSELB handles `aws_elb` (Classic Load Balancer).
type AWSELB struct{}

func (AWSELB) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	return awsELBRequest(r.Region, "Load Balancer"), nil
}

func (AWSELB) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	return parseELB(req, raw)
}

// awsELBRequest builds the shared AWSELB price request for a given product
// family. locationType=AWS Region pins the standard (non-Outposts) SKU.
func awsELBRequest(region, family string) pricing.PriceRequest {
	return awsPriceRequest("AWSELB", region,
		awsFilter("location", awsLocation(region)),
		awsFilter("locationType", "AWS Region"),
		awsFilter("productFamily", family),
	)
}

// parseELB reads the fixed hourly LoadBalancerUsage rate and turns it into a
// monthly CostComponent, shared by AWSLB and AWSELB.
func parseELB(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	// The ELB family returns several SKUs (LoadBalancerUsage + LCU/data
	// processing); pin the fixed hourly one.
	price, err := parseAWSPriceListMatching(raw, "LoadBalancerUsage")
	if err != nil {
		return nil, err
	}
	family := filterValue(req, "productFamily")
	return []output.CostComponent{{
		Name:        fmt.Sprintf("%s hourly (base, excl. LCU/data)", awsLBLabel(family)),
		Unit:        "HOUR",
		HourlyCost:  price.USD,
		MonthlyCost: awsHourlyToMonthly(price.USD),
		Currency:    awsCurrency,
	}}, nil
}

// awsLBFamily maps the Terraform aws_lb load_balancer_type to the Price List
// productFamily. Returns "" for an unknown type.
func awsLBFamily(lbType string) string {
	switch lbType {
	case "application":
		return "Load Balancer-Application"
	case "network":
		return "Load Balancer-Network"
	case "gateway":
		return "Load Balancer-Gateway"
	default:
		return ""
	}
}

// awsLBLabel gives a short human label for a load-balancer productFamily.
func awsLBLabel(family string) string {
	switch family {
	case "Load Balancer-Application":
		return "ALB"
	case "Load Balancer-Network":
		return "NLB"
	case "Load Balancer-Gateway":
		return "GWLB"
	case "Load Balancer":
		return "Classic LB"
	default:
		return "Load Balancer"
	}
}
