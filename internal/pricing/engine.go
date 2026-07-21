// Package pricing wraps Tencent Cloud pricing APIs with a common request/response
// shape and a local cache.
//
// Each per-type Mapper produces a PriceRequest with a Product/Action pair;
// the engine routes to the right SDK client, executes InquiryPriceXxx,
// caches the raw JSON response keyed by sha256(request) namespaced by site,
// and returns it.
//
// Site selection: Tencent Cloud runs two independent sites (Chinese-mainland
// and International) chosen by the credential, not the region. The engine
// applies Config.Site via the SDK's RootDomain and isolates the cache per site
// so the two never share entries. See Config.Site and rootDomainForSite.
package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	tcCommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcErrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	tcProfile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

type Config struct {
	SecretID  string
	SecretKey string
	Region    string
	CachePath string // BoltDB file path; empty = no cache
	NoCache   bool   // when true, cache is disabled even if CachePath is set

	// Site selects which Tencent Cloud site the credential belongs to.
	//
	// Tencent Cloud runs two fully independent sites with separate account
	// systems: the Chinese-mainland site (api host <product>.tencentcloudapi.com)
	// and the International site (<product>.intl.tencentcloudapi.com). A given
	// SecretID/SecretKey pair is registered on exactly ONE of them, so the site
	// is NOT derivable from the region (both sites expose overlapping region
	// names such as ap-guangzhou / ap-singapore). It must be selected explicitly
	// to match the credential.
	//
	// Accepted values (case-insensitive, whitespace-trimmed):
	//   "" | "domestic" | "cn" | "china"          -> Chinese-mainland site (default)
	//   "intl" | "international" | "global"        -> International site
	// Any other non-empty value is treated as a literal root domain override
	// (e.g. a private-cloud gateway), passed through to the SDK unchanged.
	Site string
}

// rootDomainForSite maps a Site selector to the SDK RootDomain suffix.
//
// Returning "" lets the SDK fall back to its built-in default
// ("tencentcloudapi.com"), which is exactly the Chinese-mainland site, so the
// default (empty Site) preserves the historical behaviour. Selecting the
// international site yields "intl.tencentcloudapi.com"; the SDK then assembles
// the per-product host as "<product>.intl.tencentcloudapi.com". Any other
// non-empty value is treated as a literal root-domain override.
func rootDomainForSite(site string) string {
	switch strings.ToLower(strings.TrimSpace(site)) {
	case "", "domestic", "cn", "china":
		return "" // SDK default -> tencentcloudapi.com (Chinese-mainland site)
	case "intl", "international", "global", "overseas":
		return "intl.tencentcloudapi.com"
	default:
		return strings.TrimSpace(site) // literal root-domain override
	}
}

// PriceRequest is the neutral request submitted by a Mapper.
//
//	Product: "cvm" | "cbs" | "clb" | "cdb" | "redis" | "postgres" |
//	         "vpc" | "mongodb" | "mariadb" | "cynosdb" | "lighthouse" |
//	         "ecm" | "sqlserver" | "dcdb" | "gaap" | ...
//	Action:  "InquiryPriceRunInstances" | "InquiryPriceCreateDisks" |
//	         "DescribeDBPrice" | "InquiryPriceCreateInstance" |
//	         "InquiryPriceCreateVpnGateway" | "InquirePriceCreateDBInstances" |
//	         "DescribePrice" | "InquirePriceCreate" |
//	         "InquirePriceCreateInstances" | "DescribePriceRunInstance" |
//	         "InquiryPriceCreateDBInstances" | "DescribeDCDBPrice" |
//	         "InquiryPriceCreateProxy" | ...
//	Region:  ap-guangzhou / ap-shanghai / ...
//	Params:  action-specific input, will be JSON-marshaled into the SDK request
type PriceRequest struct {
	Product string
	Action  string
	Region  string
	Params  map[string]interface{}
}

func (r PriceRequest) CacheKey() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal cache key: %w", err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

type Engine struct {
	cfg     Config
	cache   *cache // optional BoltDB
	mu      sync.Mutex
	clients map[string]interface{} // product:region -> typed SDK client
}

func NewEngine(cfg Config) (*Engine, error) {
	if cfg.SecretID == "" || cfg.SecretKey == "" {
		return nil, errors.New("missing TENCENTCLOUD_SECRET_ID / TENCENTCLOUD_SECRET_KEY")
	}
	e := &Engine{cfg: cfg, clients: map[string]interface{}{}}
	if cfg.CachePath != "" && !cfg.NoCache {
		c, err := openCache(cfg.CachePath)
		if err != nil {
			// Cache is an optimization, not a correctness requirement. If another
			// process holds the lock or the path is unusable, degrade gracefully
			// to an uncached engine rather than failing the whole cost run.
			return e, nil
		}
		e.cache = c
	}
	return e, nil
}

func (e *Engine) Close() error {
	if e.cache != nil {
		return e.cache.Close()
	}
	return nil
}

// Query dispatches to the right SDK client and returns the raw response JSON.
// The per-type Mapper decodes it into typed CostComponents.
func (e *Engine) Query(req PriceRequest) ([]byte, error) {
	// The cache key must include the site: the Chinese-mainland and
	// International sites return different prices (and possibly currencies) for
	// an otherwise identical request, so their responses must never collide.
	// A key failure is non-fatal — we simply run uncached for this request.
	key, keyErr := e.cacheKey(req)

	if e.cache != nil && keyErr == nil {
		if hit, ok := e.cache.Get(key); ok {
			return hit, nil
		}
	}

	region := req.Region
	if region == "" {
		region = e.cfg.Region
	}

	h, ok := handlers[req.Product]
	if !ok {
		return nil, fmt.Errorf("unsupported product %q (register a productHandler in handlers.go)", req.Product)
	}
	resp, err := e.invoke(h, region, req)
	if err != nil {
		return nil, err
	}

	if e.cache != nil && keyErr == nil {
		_ = e.cache.Put(key, resp)
	}
	return resp, nil
}

// cacheKey derives the on-disk cache key for a request under the engine's
// current site. It namespaces the request's own CacheKey() with the normalized
// site root domain so responses from different sites never collide.
func (e *Engine) cacheKey(req PriceRequest) (string, error) {
	base, err := req.CacheKey()
	if err != nil {
		return "", err
	}
	// rootDomainForSite("") == "" (Chinese-mainland default); using it as a
	// prefix keeps pre-existing domestic keys stable while isolating intl.
	return rootDomainForSite(e.cfg.Site) + "|" + base, nil
}

// clientFn builds a typed SDK client from a credential/profile pair.
type clientFn func(*tcCommon.Credential, *tcProfile.ClientProfile) (interface{}, error)

// client returns a cached SDK client for the given product/region, creating it
// on first use. Tencent SDK clients are safe for concurrent use, so a single
// instance is shared across goroutines.
func (e *Engine) client(product, region string, newFn clientFn) (interface{}, error) {
	key := product + ":" + region
	e.mu.Lock()
	if c, ok := e.clients[key]; ok {
		e.mu.Unlock()
		return c, nil
	}
	e.mu.Unlock()

	credential := tcCommon.NewCredential(e.cfg.SecretID, e.cfg.SecretKey)
	prof := tcProfile.NewClientProfile()
	// Select the site (domestic vs international) via the SDK's RootDomain, which
	// the SDK combines with the product name to build "<product>.<rootDomain>".
	// We deliberately do NOT set HttpProfile.Endpoint here: Endpoint is a full
	// host that takes precedence over RootDomain and would pin every product to
	// the Chinese-mainland site, defeating international-site credentials.
	if rd := rootDomainForSite(e.cfg.Site); rd != "" {
		prof.HttpProfile.RootDomain = rd
	}

	c, err := newFn(credential, prof)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if existing, ok := e.clients[key]; ok {
		return existing, nil // lost the race; reuse the winner
	}
	e.clients[key] = c
	return c, nil
}

// invoke resolves the cached SDK client for a product handler, looks up the
// action, and executes it. This is the single generic dispatch path shared by
// every product; per-product knowledge lives entirely in handlers.go.
func (e *Engine) invoke(h productHandler, region string, req PriceRequest) ([]byte, error) {
	raw, err := e.client(h.product, region, func(cred *tcCommon.Credential, prof *tcProfile.ClientProfile) (interface{}, error) {
		return h.newClient(cred, region, prof)
	})
	if err != nil {
		return nil, err
	}
	action, ok := h.actions[req.Action]
	if !ok {
		return nil, fmt.Errorf("unsupported %s action %q", h.product, req.Action)
	}
	return action(raw, req.Params)
}

// ----- helpers -----

// bindParams marshals a neutral map into a typed SDK request by round-tripping JSON.
// Mappers keep everything as string-keyed maps; the SDK request struct fields are
// tagged with `json:"Xxx"` so this works transparently.
func bindParams(params map[string]interface{}, target interface{}) error {
	blob, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(blob, target); err != nil {
		return fmt.Errorf("bind params: %w", err)
	}
	return nil
}

// jsonStringer is implemented by every Tencent Cloud SDK response type.
type jsonStringer interface {
	ToJsonString() string
}

func sdkResult(out jsonStringer, err error) ([]byte, error) {
	if err != nil {
		var apiErr *tcErrors.TencentCloudSDKError
		if errors.As(err, &apiErr) {
			return nil, fmt.Errorf("tencent api %s: %s", apiErr.GetCode(), apiErr.GetMessage())
		}
		return nil, err
	}
	return []byte(out.ToJsonString()), nil
}
