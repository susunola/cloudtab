// Package resources maps Terraform resource types to per-product Mappers.
//
// Adding support for a new resource type = writing one file that implements
// Mapper.Extract (plan -> PriceRequest) and Mapper.Parse (raw API response ->
// CostComponents).
package resources

import (
	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// Mapper is the contract every resource type implements.
type Mapper interface {
	Extract(r parser.PlannedResource) (pricing.PriceRequest, error)
	Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error)
}

// StaticMapper is an optional extension for resources whose price cannot be
// fetched from a Tencent InquiryPrice* API. When a Mapper implements this
// interface, priceReport uses Estimate directly and skips the pricing engine.
type StaticMapper interface {
	Mapper
	Estimate(r parser.PlannedResource) ([]output.CostComponent, error)
}

type Registry struct {
	m map[string]Mapper
}

func (r *Registry) Register(tfType string, m Mapper) { r.m[tfType] = m }
func (r *Registry) Lookup(tfType string) (Mapper, bool) {
	m, ok := r.m[tfType]
	return m, ok
}

// DefaultRegistry ships with the initial supported resource types.
func DefaultRegistry() *Registry {
	r := &Registry{m: map[string]Mapper{}}
	r.Register("tencentcloud_instance", &CVMInstance{})
	r.Register("tencentcloud_cbs_storage", &CBSStorage{})
	r.Register("tencentcloud_eip", &EIP{})
	r.Register("tencentcloud_clb_instance", &CLBInstance{})
	r.Register("tencentcloud_mysql_instance", &MySQLInstance{})
	r.Register("tencentcloud_redis_instance", &RedisInstance{})
	// TODO: tencentcloud_cos_bucket, tencentcloud_cdn_domain (static price table)
	return r
}
