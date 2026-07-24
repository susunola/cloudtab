// Package pricing — AWS pricing backend.
//
// This file is the single place where AWS Price List (Pricing) SDK knowledge
// lives, mirroring the role handlers.go plays for Tencent Cloud. The Engine
// delegates any PriceRequest whose Provider is "aws" to this backend.
//
// The AWS Price List API works very differently from Tencent's per-product
// InquiryPrice* methods: there is ONE GetProducts operation, parameterised by a
// ServiceCode (e.g. "AmazonEC2") plus a set of attribute Filters (instanceType,
// location, ...). It returns PriceList: a slice of JSON strings, each a full
// product price document. We hand that slice back to the Mapper (as a JSON
// array) which extracts the OnDemand price it needs.
//
// Contract with AWS mappers (see the aws_*.go mappers in internal/resources):
//   - req.Product  = the AWS ServiceCode (required), e.g. "AmazonEC2".
//   - req.Region   = an AWS region (us-east-1, ...) — informational only; the
//     mapper is responsible for translating it into a "location" filter value,
//     because the Pricing API filters on the human-readable location name
//     ("US East (N. Virginia)"), not the region code.
//   - req.Params["Filters"] = []interface{} of map[string]interface{} each with
//     "Field" and "Value" (both strings). All filters are matched with
//     TERM_MATCH. ServiceCode is added automatically from req.Product.
//   - req.Params["MaxResults"] = optional float64/int cap (default 100).
//
// The backend returns the PriceList JSON-array bytes: `[ "<product-json>", ... ]`.
package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// awsPricingAPIRegion is the region the Price List API endpoint is served from.
// The API is global but only reachable via a small set of endpoints; us-east-1
// is the canonical one and always returns prices for every region (the region
// being priced is expressed through a "location" filter, not the endpoint).
const awsPricingAPIRegion = "us-east-1"

// awsGetProductsAPI is the subset of the AWS pricing client we use. Defining it
// as an interface keeps the backend unit-testable without network access.
type awsGetProductsAPI interface {
	GetProducts(ctx context.Context, in *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

// awsBackend implements backend using the AWS Price List API.
type awsBackend struct {
	client awsGetProductsAPI
	// timeout bounds a single GetProducts round-trip so a stalled pricing call
	// cannot hang the whole cost run.
	timeout time.Duration
}

// newAWSBackend builds the AWS pricing backend. Credentials are resolved with
// the standard AWS chain (environment, shared config/credentials files, IAM
// role, ...). When AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY are present in the
// environment they take effect through LoadDefaultConfig automatically; we also
// honour an explicit static pair if the Config carries one (AWSAccessKeyID /
// AWSSecretAccessKey), which makes the backend easy to drive from tests and
// keeps AWS creds separate from the Tencent SecretID/SecretKey.
//
// This is only ever called on the first AWS request (lazily, from
// Engine.awsBackend), so a pure-Tencent run never resolves AWS credentials.
func newAWSBackend(cfg Config) (backend, error) {
	// Bound credential resolution (including IMDS fallback) so an unreachable
	// EC2 metadata service cannot hang the whole cost run.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout())
	defer cancel()
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(awsPricingAPIRegion),
	}
	if cfg.AWSAccessKeyID != "" && cfg.AWSSecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSSessionToken),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return &awsBackend{
		client:  pricing.NewFromConfig(awsCfg),
		timeout: cfg.requestTimeout(),
	}, nil
}

// query runs a single GetProducts call built from the request's ServiceCode and
// Filters, paginating until MaxResults products are collected, and returns the
// gathered PriceList as a JSON array of product-document strings.
func (b *awsBackend) query(req PriceRequest) ([]byte, error) {
	if req.Product == "" {
		return nil, fmt.Errorf("aws: PriceRequest.Product (ServiceCode) is required")
	}
	filters, err := buildAWSFilters(req.Product, req.Params)
	if err != nil {
		return nil, err
	}

	maxResults := awsMaxResults(req.Params)

	ctx := context.Background()
	if b.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.timeout)
		defer cancel()
	}

	in := &pricing.GetProductsInput{
		ServiceCode:   aws.String(req.Product),
		Filters:       filters,
		FormatVersion: aws.String("aws_v1"),
		MaxResults:    aws.Int32(int32(maxResults)),
	}

	collected := make([]json.RawMessage, 0, maxResults)
	var token *string
	for {
		in.NextToken = token
		out, err := b.client.GetProducts(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("aws GetProducts %s: %w", req.Product, err)
		}
		for _, p := range out.PriceList {
			collected = append(collected, json.RawMessage(p))
			if len(collected) >= maxResults {
				break
			}
		}
		if len(collected) >= maxResults || out.NextToken == nil || *out.NextToken == "" {
			break
		}
		token = out.NextToken
	}

	resp, err := json.Marshal(collected)
	if err != nil {
		return nil, fmt.Errorf("aws: marshal price list: %w", err)
	}
	return resp, nil
}

// buildAWSFilters turns the neutral Params["Filters"] list into TERM_MATCH
// SDK filters, always prepending the ServiceCode filter so callers cannot forget
// it. Each entry must be a map with string "Field" and "Value".
func buildAWSFilters(serviceCode string, params map[string]interface{}) ([]pricingtypes.Filter, error) {
	filters := []pricingtypes.Filter{{
		Type:  pricingtypes.FilterTypeTermMatch,
		Field: aws.String("ServiceCode"),
		Value: aws.String(serviceCode),
	}}
	if params == nil {
		return filters, nil
	}
	raw, ok := params["Filters"]
	if !ok {
		return filters, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("aws: Params[\"Filters\"] must be a list, got %T", raw)
	}
	for i, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("aws: filter[%d] must be a map, got %T", i, item)
		}
		field, _ := m["Field"].(string)
		value, _ := m["Value"].(string)
		if field == "" {
			return nil, fmt.Errorf("aws: filter[%d] missing Field", i)
		}
		filters = append(filters, pricingtypes.Filter{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String(field),
			Value: aws.String(value),
		})
	}
	return filters, nil
}

// awsMaxResults reads an optional result cap from Params, defaulting to 100
// (also the AWS per-page maximum). Values are clamped to [1, 100] so a single
// GetProducts page satisfies the common "one matching SKU" case.
func awsMaxResults(params map[string]interface{}) int {
	const def = 100
	if params == nil {
		return def
	}
	v, ok := params["MaxResults"]
	if !ok {
		return def
	}
	n := def
	switch t := v.(type) {
	case int:
		n = t
	case int32:
		n = int(t)
	case int64:
		n = int(t)
	case float64:
		n = int(t)
	case string:
		if parsed, err := strconv.Atoi(t); err == nil {
			n = parsed
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 100 {
		n = 100
	}
	return n
}
