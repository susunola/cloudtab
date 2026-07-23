package resources

// AWS ElastiCache node pricing — Terraform `aws_elasticache_cluster`.
//
// ServiceCode: AmazonElastiCache, productFamily "Cache Instance". The
// on-demand hourly rate for one cache node is pinned by instanceType (the
// cache.* node type), cacheEngine (Redis / Memcached) and location. A cluster
// runs `num_cache_nodes` of them, so the monthly figure is per-node hourly ×
// 730 × node count.

import (
	"fmt"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// AWSElastiCacheCluster handles `aws_elasticache_cluster`.
type AWSElastiCacheCluster struct{}

func (AWSElastiCacheCluster) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
	nodeType := getStr(r.After, "node_type")
	if nodeType == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_elasticache_cluster: missing node_type")
	}
	engine := awsCacheEngine(getStr(r.After, "engine"))
	if engine == "" {
		return pricing.PriceRequest{}, fmt.Errorf("aws_elasticache_cluster: unsupported engine %q", getStr(r.After, "engine"))
	}
	nodes := getInt(r.After, "num_cache_nodes")
	if nodes <= 0 {
		nodes = 1 // Terraform defaults num_cache_nodes to 1
	}
	req := awsPriceRequest("AmazonElastiCache", r.Region,
		awsFilter("instanceType", nodeType),
		awsFilter("location", awsLocation(r.Region)),
		awsFilter("cacheEngine", engine),
		awsFilter("productFamily", "Cache Instance"),
	)
	// Stash node count so Parse multiplies per-node price by it.
	req.Params["Quantity"] = nodes
	return req, nil
}

func (AWSElastiCacheCluster) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
	nodeType := filterValue(req, "instanceType")
	engine := filterValue(req, "cacheEngine")
	nodes := awsQuantity(req)
	if nodes <= 0 {
		nodes = 1
	}
	name := fmt.Sprintf("ElastiCache %s %s", engine, nodeType)
	if nodes > 1 {
		name = fmt.Sprintf("ElastiCache %s %s x%d", engine, nodeType, nodes)
	}
	return awsScaledCost(name, float64(nodes), raw)
}

// awsCacheEngine maps the Terraform `engine` value to the Price List
// "cacheEngine" attribute. Terraform uses "redis"|"memcached"|"valkey"; the
// Price List uses "Redis"|"Memcached". Valkey nodes are priced on the Redis
// node rate, so we map valkey→Redis for a close estimate.
func awsCacheEngine(engine string) string {
	switch engine {
	case "redis", "valkey", "":
		// ElastiCache defaults to Redis-family pricing; empty engine (rare in
		// a plan) falls back to Redis rather than failing.
		return "Redis"
	case "memcached":
		return "Memcached"
	default:
		return ""
	}
}
