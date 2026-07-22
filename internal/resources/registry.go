// Package resources maps Terraform resource types to per-product Mappers.
//
// Adding support for a new resource type = writing one file that implements
// Mapper.Extract (plan -> PriceRequest) and Mapper.Parse (raw API response ->
// CostComponents).
package resources

import (
	"sync"

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

// defaultRegistryOnce guards the singleton initialisation of the built-in
// registry so that repeated calls to DefaultRegistry return the same *Registry.
var defaultRegistryOnce sync.Once
var defaultRegistryInstance *Registry

// DefaultRegistry returns a shared Registry pre-loaded with all supported
// resource types. It is safe for concurrent use.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		r := &Registry{m: map[string]Mapper{}}
		r.Register("tencentcloud_instance", &CVMInstance{})
		r.Register("tencentcloud_cbs_storage", &CBSStorage{})
		r.Register("tencentcloud_eip", &EIP{})
		r.Register("tencentcloud_clb_instance", &CLBInstance{})
		r.Register("tencentcloud_mysql_instance", &MySQLInstance{})
		r.Register("tencentcloud_postgresql_instance", &PostgreSQLInstance{})
		r.Register("tencentcloud_redis_instance", &RedisInstance{})
		r.Register("tencentcloud_vpn_gateway", &VPNGateway{})
		r.Register("tencentcloud_mongodb_instance", &MongoDBInstance{})
		r.Register("tencentcloud_mariadb_instance", &MariaDBInstance{})
		r.Register("tencentcloud_cynosdb_cluster", &CynosDBCluster{})
		r.Register("tencentcloud_lighthouse_instance", &LighthouseInstance{})
		r.Register("tencentcloud_ecm_instance", &ECMInstance{})
		r.Register("tencentcloud_sqlserver_instance", &SQLServerInstance{})
		r.Register("tencentcloud_dcdb_instance", &DCDBInstance{})
		r.Register("tencentcloud_gaap_proxy", &GAAPProxy{})
		r.Register("tencentcloud_cwp_license_order", &YunjingLicense{})
		r.Register("tencentcloud_cloudhsm_instance", &CloudHSMInstance{})
		r.Register("tencentcloud_domain_registration", &DomainRegistration{})
		// TODO: tencentcloud_cos_bucket, tencentcloud_cdn_domain (static price table)

		// --- AWS (priced via the AWS Price List backend) ---
		r.Register("aws_instance", &AWSInstance{})
		r.Register("aws_ebs_volume", &AWSEBSVolume{})
		r.Register("aws_db_instance", &AWSDBInstance{})
		r.Register("aws_elasticache_cluster", &AWSElastiCacheCluster{})
		r.Register("aws_lb", &AWSLB{})
		r.Register("aws_elb", &AWSELB{})
		// NOTE: aws_s3_bucket and aws_eip are intentionally NOT registered.
		// Their cost is purely usage-driven (S3: GB stored / requests / egress;
		// EIP: only billed when idle/unattached or as a public-IPv4 hourly
		// charge). A Terraform plan carries none of those usage figures, so any
		// monthly number would be fabricated. See docs/design.md.
		defaultRegistryInstance = r
	})
	return defaultRegistryInstance
}
