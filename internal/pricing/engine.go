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

	cbs "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cbs/v20170312"
	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	tcCommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcErrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	tcProfile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type Config struct {
	SecretID  string
	SecretKey string
	Region    string
	CachePath string // BoltDB file path; empty = no cache
}

// PriceRequest is the neutral request submitted by a Mapper.
//
//	Product: "cvm" | "cbs" | "clb" | "cdb" | "redis" | ...
//	Action:  "InquiryPriceRunInstances" | "InquiryPriceCreateDisks" | ...
//	Region:  ap-guangzhou / ap-shanghai / ...
//	Params:  action-specific input, will be JSON-marshaled into the SDK request
type PriceRequest struct {
	Product string
	Action  string
	Region  string
	Params  map[string]interface{}
}

func (r PriceRequest) CacheKey() string {
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
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
	if cfg.CachePath != "" {
		c, err := openCache(cfg.CachePath)
		if err != nil {
			return nil, fmt.Errorf("open cache: %w", err)
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
		if hit, ok := e.cache.Get(req.CacheKey()); ok {
			return hit, nil
		}
	}
	region := req.Region
	if region == "" {
		region = e.cfg.Region
	}

	var (
		resp []byte
		err  error
	)
	switch req.Product {
	case "cvm":
		resp, err = e.queryCVM(region, req)
	case "cbs":
		resp, err = e.queryCBS(region, req)
	case "clb":
		resp, err = e.queryCLB(region, req)
	default:
		return nil, fmt.Errorf("unsupported product %q (add a handler in engine.go)", req.Product)
	}
	if err != nil {
		return nil, err
	}
	if e.cache != nil {
		_ = e.cache.Put(req.CacheKey(), resp)
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

// ----- per-product dispatch -----

func (e *Engine) queryCVM(region string, req PriceRequest) ([]byte, error) {
	raw, err := e.client("cvm", region, func(cred *tcCommon.Credential, prof *tcProfile.ClientProfile) (interface{}, error) {
		return cvm.NewClient(cred, region, prof)
	})
	if err != nil {
		return nil, err
	}
	client := raw.(*cvm.Client)

	switch req.Action {
	case "InquiryPriceRunInstances":
		in := cvm.NewInquiryPriceRunInstancesRequest()
		if err := bindParams(req.Params, in); err != nil {
			return nil, err
		}
		out, err := client.InquiryPriceRunInstances(in)
		return sdkResult(out, err)
	default:
		return nil, fmt.Errorf("unsupported cvm action %q", req.Action)
	}
}

func (e *Engine) queryCBS(region string, req PriceRequest) ([]byte, error) {
	raw, err := e.client("cbs", region, func(cred *tcCommon.Credential, prof *tcProfile.ClientProfile) (interface{}, error) {
		return cbs.NewClient(cred, region, prof)
	})
	if err != nil {
		return nil, err
	}
	client := raw.(*cbs.Client)

	switch req.Action {
	case "InquiryPriceCreateDisks":
		in := cbs.NewInquiryPriceCreateDisksRequest()
		if err := bindParams(req.Params, in); err != nil {
			return nil, err
		}
		out, err := client.InquiryPriceCreateDisks(in)
		return sdkResult(out, err)
	default:
		return nil, fmt.Errorf("unsupported cbs action %q", req.Action)
	}
}

func (e *Engine) queryCLB(region string, req PriceRequest) ([]byte, error) {
	raw, err := e.client("clb", region, func(cred *tcCommon.Credential, prof *tcProfile.ClientProfile) (interface{}, error) {
		return clb.NewClient(cred, region, prof)
	})
	if err != nil {
		return nil, err
	}
	client := raw.(*clb.Client)

	switch req.Action {
	case "InquiryPriceCreateLoadBalancer":
		in := clb.NewInquiryPriceCreateLoadBalancerRequest()
		if err := bindParams(req.Params, in); err != nil {
			return nil, err
		}
		out, err := client.InquiryPriceCreateLoadBalancer(in)
		return sdkResult(out, err)
	default:
		return nil, fmt.Errorf("unsupported clb action %q", req.Action)
	}
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
