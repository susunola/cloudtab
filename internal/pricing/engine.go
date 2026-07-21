// Package pricing wraps Tencent Cloud pricing APIs with a common request/response
// shape and a local cache.
//
// Each per-type Mapper produces a PriceRequest with a Product/Action pair;
// the engine routes to the right SDK client, executes InquiryPriceXxx,
// caches the raw JSON response keyed by sha256(request), and returns it.
package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	if e.cache != nil {
		key, err := req.CacheKey()
		if err != nil {
			return nil, err
		}
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
	if e.cache != nil {
		key, kerr := req.CacheKey()
		if kerr != nil {
			// Cache key failure is non-fatal; just skip caching this response.
			return resp, nil
		}
		_ = e.cache.Put(key, resp)
	}
	return resp, nil
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
	prof.HttpProfile.Endpoint = product + ".tencentcloudapi.com"

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
