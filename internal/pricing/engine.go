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
	"time"

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

	// AWS credentials for the AWS Price List backend. These are OPTIONAL and
	// entirely separate from the Tencent SecretID/SecretKey above. When left
	// empty, the AWS SDK's default credential chain is used (environment vars
	// AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN, shared
	// config files, IAM role, ...). They are only consulted when an AWS
	// resource is actually priced, so a pure-Tencent run needs none of them.
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSSessionToken    string

	// Alibaba Cloud credentials for the BSS pricing backend. These are OPTIONAL
	// and only consulted when an Alibaba Cloud resource is priced. When left
	// empty, the environment vars ALIBABA_ACCESS_KEY_ID / ALIBABA_ACCESS_KEY_SECRET
	// are used.
	AlibabaAccessKeyID     string
	AlibabaAccessKeySecret string

	// Huawei Cloud credentials for the BSS pricing backend. These are OPTIONAL
	// and only consulted when a Huawei Cloud resource is priced. When left
	// empty, the environment vars HUAWEI_ACCESS_KEY_ID / HUAWEI_SECRET_ACCESS_KEY
	// are used.
	HuaweiAccessKeyID     string
	HuaweiSecretAccessKey string

	// HuaweiProjectID is the UUID project ID sent as RateOnDemandReq.ProjectId
	// for Huawei Cloud pricing. It is NOT the region. When empty, no project_id
	// is sent (the API then bills under the credential's default project). Read
	// from HUAWEI_PROJECT_ID env in the CLI.
	HuaweiProjectID string

	// Timeout bounds a single pricing round-trip (per attempt) so a stalled
	// InquiryPrice call cannot hang the whole cost run. It is applied to both
	// backends: for Tencent Cloud via the SDK profile's HttpProfile.ReqTimeout
	// (whole-second granularity), and for AWS via a context deadline. Zero or
	// negative means "use defaultRequestTimeout".
	Timeout time.Duration

	// MaxRetries is the number of ADDITIONAL attempts made after the first when
	// a request fails with a retryable error (rate limiting, request timeout,
	// transient network/5xx). Non-retryable errors (bad params, unknown SKU)
	// fail immediately. Zero means "use defaultMaxRetries"; a negative value
	// disables retries entirely.
	MaxRetries int

	// CacheTTL sets how long a successful pricing response stays in the on-disk
	// cache before being treated as stale. Zero means "use defaultTTL" (24h).
	// A shorter TTL keeps prices fresher at the cost of more API calls.
	CacheTTL time.Duration
}

const (
	// defaultRequestTimeout mirrors the AWS backend's original 30s bound and is
	// now applied to Tencent calls as well so neither backend can hang forever.
	defaultRequestTimeout = 30 * time.Second
	// defaultMaxRetries retries a couple of times on transient/rate-limit
	// errors, which is enough to ride out a brief InquiryPrice QPS spike without
	// materially lengthening a healthy run.
	defaultMaxRetries = 2
	// retryBaseBackoff is the first backoff; it doubles each attempt (capped).
	retryBaseBackoff = 200 * time.Millisecond
	retryMaxBackoff  = 2 * time.Second
)

// requestTimeout resolves the effective per-attempt timeout.
func (c Config) requestTimeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultRequestTimeout
}

// maxRetries resolves the effective retry count. A negative Config value means
// "no retries" (0 additional attempts); zero means "use the default".
func (c Config) maxRetries() int {
	if c.MaxRetries < 0 {
		return 0
	}
	if c.MaxRetries == 0 {
		return defaultMaxRetries
	}
	return c.MaxRetries
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
//	Provider: "" | "tencentcloud" (default) | "aws". Selects the pricing
//	          backend. Empty is treated as "tencentcloud" for backward
//	          compatibility, so all pre-existing mappers keep working unchanged.
//	Product:  Tencent: "cvm" | "cbs" | "clb" | "cdb" | "redis" | ...
//	          AWS: the Price List ServiceCode, e.g. "AmazonEC2" | "AmazonRDS" |
//	          "AmazonElastiCache" | "AWSELB" | "AmazonS3".
//	Action:   Tencent: "InquiryPriceRunInstances" | "DescribeDBPrice" | ...
//	          AWS: unused (the AWS backend prices via Filters in Params); may be
//	          left empty or set to a descriptive label.
//	Region:   Tencent: ap-guangzhou / ap-shanghai / ...
//	          AWS: us-east-1 / eu-west-1 / ... (used only to build a Location
//	          filter; the Pricing API endpoint itself is always us-east-1).
//	Params:   action-specific input. Tencent: JSON-marshaled into the SDK
//	          request. AWS: a neutral map the AWS backend turns into GetProducts
//	          filters (see aws_backend.go).
type PriceRequest struct {
	Provider string
	Product  string
	Action   string
	Region   string
	Params   map[string]interface{}
}

// provider returns the request's provider, defaulting to Tencent Cloud when
// unset so that every mapper written before multi-cloud support keeps routing
// to the Tencent backend without changes.
func (r PriceRequest) provider() string {
	p := strings.ToLower(strings.TrimSpace(r.Provider))
	if p == "" {
		return providerTencent
	}
	return p
}

const (
	providerTencent  = "tencentcloud"
	providerAWS      = "aws"
	providerAlibaba  = "alibaba"
	providerHuawei   = "huawei"
)

func (r PriceRequest) CacheKey() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal cache key: %w", err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// backend is a provider-specific pricing source. The Tencent path is
// implemented directly on *Engine (client/invoke/handlers) for historical
// reasons and to keep its extensive test surface stable; additional providers
// (e.g. AWS) plug in as a backend the Engine delegates to from Query.
type backend interface {
	// query prices a single request and returns the raw provider response
	// bytes, which the caller's Mapper decodes into CostComponents.
	query(req PriceRequest) ([]byte, error)
}

type Engine struct {
	cfg     Config
	cache   *cache // optional BoltDB
	mu      sync.Mutex
	clients map[string]interface{} // product:region -> typed SDK client

	// aws is the lazily-initialised AWS pricing backend. It is created on the
	// first AWS request (so a pure-Tencent run never touches the AWS SDK or
	// requires AWS credentials). Guarded by awsOnce.
	awsOnce sync.Once
	aws     backend
	awsErr  error

	// alibaba is the lazily-initialised Alibaba Cloud BSS pricing backend.
	alibabaOnce sync.Once
	alibaba     backend
	alibabaErr  error

	// huawei is the lazily-initialised Huawei Cloud BSS pricing backend.
	huaweiOnce sync.Once
	huawei     backend
	huaweiErr  error

	// inflight de-duplicates concurrent identical requests within this process:
	// when several goroutines ask for the same cache key at once (a plan with
	// many identically-specced resources on a cold cache), only the first does
	// the real backend call; the rest wait on its result. This is a minimal,
	// dependency-free equivalent of singleflight (we deliberately avoid pulling
	// in golang.org/x/sync to preserve the zero-dependency-drift invariant).
	flightMu sync.Mutex
	flight   map[string]*inflightCall
}

// inflightCall is a single de-duplicated request in progress. Waiters block on
// done, then read the shared result.
type inflightCall struct {
	done chan struct{}
	resp []byte
	err  error
}

func NewEngine(cfg Config) (*Engine, error) {
	// Tencent Cloud credentials are no longer required up front: a plan that
	// only prices AWS / Alibaba / Huawei resources needs none of them. We
	// validate the Tencent SecretID/SecretKey lazily inside dispatch() only
	// when a Tencent Cloud resource is actually priced (code review #3).
	e := &Engine{cfg: cfg, clients: map[string]interface{}{}, flight: map[string]*inflightCall{}}
	if cfg.CachePath != "" && !cfg.NoCache {
		c, err := openCache(cfg.CachePath, cfg.CacheTTL)
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
//
// The request path is: on-disk cache -> in-flight de-duplication -> backend
// dispatch (with retry). The cache and dedup layers key off the same
// site-namespaced cache key so a healthy run touches each distinct SKU at most
// once, keeping us well under the InquiryPrice QPS limit.
func (e *Engine) Query(req PriceRequest) ([]byte, error) {
	// The cache key must include the site: the Chinese-mainland and
	// International sites return different prices (and possibly currencies) for
	// an otherwise identical request, so their responses must never collide.
	// A key failure is non-fatal — we simply run uncached (and un-deduped) for
	// this request.
	key, keyErr := e.cacheKey(req)
	keyed := keyErr == nil

	if e.cache != nil && keyed {
		if hit, ok := e.cache.Get(key); ok {
			return hit, nil
		}
	}

	// Without a usable key we cannot safely de-duplicate or cache; just run.
	if !keyed {
		return e.dispatchWithRetry(req)
	}

	resp, err := e.do(key, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// do performs a keyed request through the in-flight de-duplication layer. The
// first caller for a key runs the real backend call (and populates the cache);
// concurrent callers for the same key block and share its result instead of
// issuing their own backend call.
func (e *Engine) do(key string, req PriceRequest) ([]byte, error) {
	e.flightMu.Lock()
	// Lazily initialise the in-flight map. NewEngine always populates it, but
	// guarding here means a directly-constructed &Engine{} (e.g. in tests or
	// future call sites) can never panic on a nil-map write.
	if e.flight == nil {
		e.flight = map[string]*inflightCall{}
	}
	if call, ok := e.flight[key]; ok {
		e.flightMu.Unlock()
		<-call.done
		return call.resp, call.err
	}
	call := &inflightCall{done: make(chan struct{})}
	e.flight[key] = call
	e.flightMu.Unlock()

	call.resp, call.err = e.dispatchWithRetry(req)

	// Populate the on-disk cache only on success so a transient failure is not
	// remembered.
	if call.err == nil && e.cache != nil {
		_ = e.cache.Put(key, call.resp)
	}

	e.flightMu.Lock()
	delete(e.flight, key)
	e.flightMu.Unlock()
	close(call.done)

	return call.resp, call.err
}

// dispatchWithRetry wraps dispatch with a bounded exponential backoff, retrying
// only errors classified as retryable (rate limiting, request timeout, or a
// transient network/5xx hiccup). Non-retryable errors — unknown product, bad
// params, unsupported action — return immediately so a mistake fails fast.
func (e *Engine) dispatchWithRetry(req PriceRequest) ([]byte, error) {
	attempts := e.cfg.maxRetries() + 1
	backoff := retryBaseBackoff
	var lastErr error
	for i := 0; i < attempts; i++ {
		resp, err := e.dispatch(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if i == attempts-1 || !isRetryable(err) {
			break
		}
		time.Sleep(backoff)
		if backoff *= 2; backoff > retryMaxBackoff {
			backoff = retryMaxBackoff
		}
	}
	return nil, lastErr
}

// dispatch routes a request to the provider-specific backend. Tencent Cloud is
// served by the Engine's own client/invoke/handlers path (unchanged); AWS is
// delegated to the lazily-created AWS backend.
func (e *Engine) dispatch(req PriceRequest) ([]byte, error) {
	switch req.provider() {
	case providerTencent:
		if e.cfg.SecretID == "" || e.cfg.SecretKey == "" {
			return nil, fmt.Errorf("tencentcloud: missing credentials (set TENCENTCLOUD_SECRET_ID / TENCENTCLOUD_SECRET_KEY or Config.SecretID / SecretKey)")
		}
		region := req.Region
		if region == "" {
			region = e.cfg.Region
		}
		h, ok := handlers[req.Product]
		if !ok {
			return nil, fmt.Errorf("unsupported product %q (register a productHandler in handlers.go)", req.Product)
		}
		return e.invoke(h, region, req)
	case providerAWS:
		b, err := e.awsBackend()
		if err != nil {
			return nil, err
		}
		resp, err := b.query(req)
		if err != nil {
			return nil, fmt.Errorf("aws %s: %w", req.Product, err)
		}
		return resp, nil
	case providerAlibaba:
		b, err := e.alibabaBackend()
		if err != nil {
			return nil, err
		}
		resp, err := b.query(req)
		if err != nil {
			return nil, fmt.Errorf("alibaba %s: %w", req.Product, err)
		}
		return resp, nil
	case providerHuawei:
		b, err := e.huaweiBackend()
		if err != nil {
			return nil, err
		}
		resp, err := b.query(req)
		if err != nil {
			return nil, fmt.Errorf("huawei %s: %w", req.Product, err)
		}
		return resp, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", req.provider())
	}
}

// isRetryable reports whether an error is worth retrying with backoff. It is
// deliberately conservative: only rate limiting, request timeouts, and clearly
// transient server/network conditions qualify. Anything else (bad params,
// unknown SKU, unsupported product/action) is a deterministic failure that a
// retry cannot fix, so it returns false and fails fast.
//
// Tencent Cloud surfaces API errors as strings via sdkResult ("tencent api
// <Code>: <Message>"); AWS SDK errors carry throttling/timeout signatures in
// their text too. Matching on well-known substrings keeps this dependency-free
// and works across both backends without importing either SDK's error types
// here.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, sig := range retryableSignatures {
		if strings.Contains(msg, sig) {
			return true
		}
	}
	return false
}

// retryableSignatures are lowercase substrings that mark a transient error.
// Tencent throttling codes take the form "RequestLimitExceeded" /
// "...LimitExceeded" / "InternalError"; AWS throttling is "Throttling" /
// "ThrottlingException" / "TooManyRequestsException". Timeouts and connection
// resets are transient on either side.
var retryableSignatures = []string{
	"limitexceeded", // Tencent RequestLimitExceeded / <X>.LimitExceeded
	"requestlimitexceeded",
	"throttl",         // AWS Throttling / ThrottlingException
	"toomanyrequests", // AWS TooManyRequestsException
	"rate exceeded",
	"internalerror",      // Tencent transient InternalError
	"serviceunavailable", // 503
	"service unavailable",
	"timeout", // context/request/dial timeout
	"timed out",
	"deadline exceeded",
	"connection reset",
	"connection refused",
	"eof", // truncated response / closed connection
	"temporary",
}

// awsBackend lazily constructs the AWS pricing backend once and reuses it. The
// AWS SDK and its credential resolution are only touched when an AWS resource
// is actually priced, so pure-Tencent runs are unaffected.
func (e *Engine) awsBackend() (backend, error) {
	e.awsOnce.Do(func() {
		e.aws, e.awsErr = newAWSBackend(e.cfg)
	})
	return e.aws, e.awsErr
}

// alibabaBackend lazily constructs the Alibaba Cloud BSS pricing backend.
func (e *Engine) alibabaBackend() (backend, error) {
	e.alibabaOnce.Do(func() {
		e.alibaba, e.alibabaErr = newAlibabaBackend(e.cfg)
	})
	return e.alibaba, e.alibabaErr
}

// huaweiBackend lazily constructs the Huawei Cloud BSS pricing backend.
func (e *Engine) huaweiBackend() (backend, error) {
	e.huaweiOnce.Do(func() {
		e.huawei, e.huaweiErr = newHuaweiBackend(e.cfg)
	})
	return e.huawei, e.huaweiErr
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
	// Bound each HTTP round-trip so a stalled InquiryPrice call cannot hang the
	// whole run. The SDK's ReqTimeout is whole-second granularity, so we round
	// up (a sub-second timeout still yields at least 1s rather than 0 = no
	// timeout). This mirrors the AWS backend's context deadline.
	prof.HttpProfile.ReqTimeout = timeoutSeconds(e.cfg.requestTimeout())

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
	resp, err := action(raw, req.Params)
	if err != nil {
		return nil, fmt.Errorf("tencentcloud %s.%s: %w", h.product, req.Action, err)
	}
	return resp, nil
}

// timeoutSeconds converts a duration into the whole-second timeout the Tencent
// SDK profile expects, rounding up so any positive duration yields at least 1s
// (0 would disable the timeout, which is exactly what we want to avoid).
func timeoutSeconds(d time.Duration) int {
	if d <= 0 {
		return int(defaultRequestTimeout / time.Second)
	}
	secs := int(d / time.Second)
	if d%time.Second != 0 {
		secs++
	}
	if secs < 1 {
		secs = 1
	}
	return secs
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
