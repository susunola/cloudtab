package resources

// AWS DynamoDB provisioned-capacity pricing — Terraform `aws_dynamodb_table`.
//
// ServiceCode: AmazonDynamoDB. We price ONLY tables in PROVISIONED billing mode,
// because that is the one mode whose cost is deterministic from a plan: you
// declare read_capacity (RCU) and write_capacity (WCU), each billed per
// capacity-unit-hour. On-demand (PAY_PER_REQUEST) tables are billed per actual
// request and storage, none of which a plan carries — those we deliberately do
// not price (returning no components rather than a fabricated number).
//
// The provisioned RCU/WCU SKUs are disambiguated by usagetype substrings:
//   - "ReadCapacityUnit-Hrs"  -> per-RCU-hour rate
//   - "WriteCapacityUnit-Hrs" -> per-WCU-hour rate
// We issue two price requests (one per capacity kind) and emit two components.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSDynamoDBTable handles `aws_dynamodb_table` (provisioned mode only).
type AWSDynamoDBTable struct{}

func (AWSDynamoDBTable) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	// Terraform defaults billing_mode to "PROVISIONED" when unset.
	mode := getStr(r.After, "billing_mode")
	if mode == "" {
		mode = "PROVISIONED"
	}
	if mode != "PROVISIONED" {
		// On-demand: usage-driven, cannot estimate from a plan. Signal "skip"
		// with an empty Product so priceReport can drop it cleanly.
		return pricing.PriceRequest{}, fmt.Errorf("aws_dynamodb_table: %s billing mode is usage-driven, not priced from a plan", mode)
	}
	rcu := getInt(r.After, "read_capacity")
	wcu := getInt(r.After, "write_capacity")
	if rcu <= 0 && wcu <= 0 {
		return pricing.PriceRequest{}, fmt.Errorf("aws_dynamodb_table: provisioned table has zero read_capacity and write_capacity")
	}
	// One request carries both capacities; Parse issues nothing further because
	// both RCU and WCU SKUs come back from the same DynamoDB product query when
	// filtered by group. We filter broadly and match usagetype in Parse.
	req := awsPriceRequest("AmazonDynamoDB", r.Region,
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("groupDescription", "DynamoDB Provisioned"),
	)
	req.Params["RCU"] = rcu
	req.Params["WCU"] = wcu
	return req, nil
}

func (AWSDynamoDBTable) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	rcu := awsParamInt(req, "RCU")
	wcu := awsParamInt(req, "WCU")

	var comps []output.CostComponent
	if rcu > 0 {
		price, err := parseAWSPriceListMatching(raw, "ReadCapacityUnit-Hrs")
		if err != nil {
			return nil, fmt.Errorf("dynamodb RCU: %w", err)
		}
		hourly := price.USD * float64(rcu)
		comps = append(comps, output.CostComponent{
			Name:        fmt.Sprintf("DynamoDB provisioned RCU x%d", rcu),
			Unit:        "HOUR",
			HourlyCost:  hourly,
			MonthlyCost: awsHourlyToMonthly(hourly),
			Currency:    awsCurrency,
		})
	}
	if wcu > 0 {
		price, err := parseAWSPriceListMatching(raw, "WriteCapacityUnit-Hrs")
		if err != nil {
			return nil, fmt.Errorf("dynamodb WCU: %w", err)
		}
		hourly := price.USD * float64(wcu)
		comps = append(comps, output.CostComponent{
			Name:        fmt.Sprintf("DynamoDB provisioned WCU x%d", wcu),
			Unit:        "HOUR",
			HourlyCost:  hourly,
			MonthlyCost: awsHourlyToMonthly(hourly),
			Currency:    awsCurrency,
		})
	}
	if len(comps) == 0 {
		return nil, fmt.Errorf("dynamodb: no provisioned capacity to price")
	}
	return comps, nil
}

// awsParamInt reads back an int stashed in Params (RCU/WCU), tolerant of the
// numeric types encoding/json and our own code may produce.
func awsParamInt(req pricing.PriceRequest, key string) int64 {
	if req.Params == nil {
		return 0
	}
	switch v := req.Params[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}
