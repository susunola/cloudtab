package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

func TestAWSElastiCacheExtract(t *testing.T) {
	r := parser.PlannedResource{
		Address: "aws_elasticache_cluster.cache",
		Type:    "aws_elasticache_cluster",
		Region:  "ap-southeast-1",
		After: map[string]interface{}{
			"node_type":       "cache.m5.large",
			"engine":          "redis",
			"num_cache_nodes": 2,
		},
	}
	req, err := AWSElastiCacheCluster{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Provider != "aws" || req.Product != "AmazonElastiCache" {
		t.Fatalf("route = %s/%s, want aws/AmazonElastiCache", req.Provider, req.Product)
	}
	if got := filterValue(req, "instanceType"); got != "cache.m5.large" {
		t.Fatalf("instanceType = %q, want cache.m5.large", got)
	}
	if got := filterValue(req, "cacheEngine"); got != "Redis" {
		t.Fatalf("cacheEngine = %q, want Redis", got)
	}
	if got := filterValue(req, "location"); got != "Asia Pacific (Singapore)" {
		t.Fatalf("location = %q, want Asia Pacific (Singapore)", got)
	}
	if got := awsQuantity(req); got != 2 {
		t.Fatalf("stashed node count = %d, want 2", got)
	}
}

func TestAWSElastiCacheExtractMemcachedDefaultNodes(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "aws_elasticache_cluster",
		Region: "us-east-1",
		After: map[string]interface{}{
			"node_type": "cache.t3.micro",
			"engine":    "memcached",
		},
	}
	req, err := AWSElastiCacheCluster{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got := filterValue(req, "cacheEngine"); got != "Memcached" {
		t.Fatalf("cacheEngine = %q, want Memcached", got)
	}
	if got := awsQuantity(req); got != 1 {
		t.Fatalf("default node count = %d, want 1", got)
	}
}

func TestAWSElastiCacheExtractMissingNodeType(t *testing.T) {
	if _, err := (AWSElastiCacheCluster{}).Extract(parser.PlannedResource{
		Type:  "aws_elasticache_cluster",
		After: map[string]interface{}{"engine": "redis"},
	}); err == nil {
		t.Fatal("expected error for missing node_type")
	}
}

func TestAWSElastiCacheParseMultiNode(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "aws_elasticache_cluster",
		Region: "us-east-1",
		After: map[string]interface{}{
			"node_type":       "cache.m5.large",
			"engine":          "redis",
			"num_cache_nodes": 3,
		},
	}
	req, err := AWSElastiCacheCluster{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	// $0.10/hr per node × 3 nodes.
	raw := []byte(`[
		{
			"product": {"attributes": {"instanceType": "cache.m5.large"}},
			"terms": {"OnDemand": {"X": {"priceDimensions": {"X.1": {
				"unit": "Hrs",
				"pricePerUnit": {"USD": "0.100000000"}
			}}}}}
		}
	]`)
	comps, err := AWSElastiCacheCluster{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	c := comps[0]
	if !almostEqAWS(c.HourlyCost, 0.30) {
		t.Fatalf("HourlyCost = %v, want 0.30 (0.10 × 3 nodes)", c.HourlyCost)
	}
	if !almostEqAWS(c.MonthlyCost, 0.30*hoursPerMonth) {
		t.Fatalf("MonthlyCost = %v, want %v", c.MonthlyCost, 0.30*hoursPerMonth)
	}
	if c.Name != "ElastiCache Redis cache.m5.large x3" {
		t.Fatalf("Name = %q, want ElastiCache Redis cache.m5.large x3", c.Name)
	}
}

func TestAWSCacheEngineMapping(t *testing.T) {
	cases := map[string]string{
		"redis":     "Redis",
		"valkey":    "Redis",
		"":          "Redis",
		"memcached": "Memcached",
		"unknown":   "",
	}
	for in, want := range cases {
		if got := awsCacheEngine(in); got != want {
			t.Errorf("awsCacheEngine(%q) = %q, want %q", in, got, want)
		}
	}
}
