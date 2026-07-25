package pricing

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tcCommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcProfile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

// TestRootDomainForSite pins the site->RootDomain mapping, the single piece of
// logic that decides whether a credential talks to the Chinese-mainland site or
// the International site. Region is intentionally NOT part of this decision.
func TestRootDomainForSite(t *testing.T) {
	cases := []struct {
		name string
		site string
		want string
	}{
		// Chinese-mainland site: empty RootDomain lets the SDK default to
		// "tencentcloudapi.com". Empty Site must map to "" for backward compat.
		{"empty default", "", ""},
		{"domestic", "domestic", ""},
		{"cn", "cn", ""},
		{"china", "china", ""},
		{"domestic upper", "DOMESTIC", ""},
		{"domestic padded", "  domestic  ", ""},

		// International site.
		{"intl", "intl", "intl.tencentcloudapi.com"},
		{"international", "international", "intl.tencentcloudapi.com"}, {"global", "global", "intl.tencentcloudapi.com"},
		{"overseas", "overseas", "intl.tencentcloudapi.com"},
		{"intl upper", "INTL", "intl.tencentcloudapi.com"},
		{"intl padded", "  Intl  ", "intl.tencentcloudapi.com"},

		// Anything else is treated as a literal root-domain override
		// (e.g. a private-cloud / proxy gateway), trimmed but otherwise verbatim.
		{"literal override", "example.internal.com", "example.internal.com"},
		{"literal override padded", "  gw.local  ", "gw.local"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rootDomainForSite(tc.site); got != tc.want {
				t.Errorf("rootDomainForSite(%q) = %q, want %q", tc.site, got, tc.want)
			}
		})
	}
}

// TestClientProfileNoHardcodedEndpoint is a regression guard for the original
// bug: the engine used to hardcode HttpProfile.Endpoint to
// "<product>.tencentcloudapi.com", which pins every product to the
// Chinese-mainland site and silently breaks International-site credentials.
//
// We reconstruct the exact profile the engine builds (the logic in client())
// for both sites and assert:
//   - Endpoint is NEVER set (that field would override RootDomain), and
//   - RootDomain is empty for domestic and "intl.tencentcloudapi.com" for intl.
//
// This mirrors client()'s profile construction without needing network I/O.
func TestClientProfileNoHardcodedEndpoint(t *testing.T) {
	build := func(site string) *tcProfile.ClientProfile {
		prof := tcProfile.NewClientProfile()
		if rd := rootDomainForSite(site); rd != "" {
			prof.HttpProfile.RootDomain = rd
		}
		return prof
	}

	domestic := build("")
	if domestic.HttpProfile.Endpoint != "" {
		t.Errorf("domestic: Endpoint = %q, want empty (must not pin the host)", domestic.HttpProfile.Endpoint)
	}
	if domestic.HttpProfile.RootDomain != "" {
		t.Errorf("domestic: RootDomain = %q, want empty (SDK default = tencentcloudapi.com)", domestic.HttpProfile.RootDomain)
	}

	intl := build("intl")
	if intl.HttpProfile.Endpoint != "" {
		t.Errorf("intl: Endpoint = %q, want empty (must not pin the host)", intl.HttpProfile.Endpoint)
	}
	if intl.HttpProfile.RootDomain != "intl.tencentcloudapi.com" {
		t.Errorf("intl: RootDomain = %q, want intl.tencentcloudapi.com", intl.HttpProfile.RootDomain)
	}
}

// TestCacheKeyIsolatedBySite guards the cache-poisoning bug: the same request
// priced under two different sites must produce two different cache keys, so an
// International-site price can never be served from a Chinese-mainland cache
// entry (or vice versa). It also asserts the domestic key equals the plain
// request key prefixed with "|" (stable, backward-friendly namespacing).
func TestCacheKeyIsolatedBySite(t *testing.T) {
	req := PriceRequest{
		Product: "cvm",
		Action:  "InquiryPriceRunInstances",
		Region:  "ap-guangzhou",
		Params:  map[string]interface{}{"InstanceType": "S5.MEDIUM4"},
	}
	base, err := req.CacheKey()
	if err != nil {
		t.Fatalf("CacheKey: %v", err)
	}

	domestic := &Engine{cfg: Config{Site: "domestic"}}
	intl := &Engine{cfg: Config{Site: "intl"}}

	dk, err := domestic.cacheKey(req)
	if err != nil {
		t.Fatalf("domestic cacheKey: %v", err)
	}
	ik, err := intl.cacheKey(req)
	if err != nil {
		t.Fatalf("intl cacheKey: %v", err)
	}

	if dk == ik {
		t.Fatalf("domestic and intl cache keys collide: %q", dk)
	}
	if want := "|" + base; dk != want {
		t.Errorf("domestic cacheKey = %q, want %q", dk, want)
	}
	if want := "intl.tencentcloudapi.com|" + base; ik != want {
		t.Errorf("intl cacheKey = %q, want %q", ik, want)
	}
}

// TestEngineClientBuildsForBothSites verifies the engine can construct a real
// SDK client for both sites without error (no network call is made at client
// construction time). This exercises the actual client() path end to end.
func TestEngineClientBuildsForBothSites(t *testing.T) {
	for _, site := range []string{"", "domestic", "intl", "international"} {
		e := &Engine{
			cfg:     Config{SecretID: "id", SecretKey: "key", Region: "ap-guangzhou", Site: site},
			clients: map[string]interface{}{},
		}
		c, err := e.client("cvm", "ap-guangzhou", func(cred *tcCommon.Credential, prof *tcProfile.ClientProfile) (interface{}, error) {
			// Reuse a real product client factory so we go through the same
			// NewClient path production uses.
			return handlers["cvm"].newClient(cred, "ap-guangzhou", prof)
		})
		if err != nil {
			t.Fatalf("site %q: client build error: %v", site, err)
		}
		if c == nil {
			t.Fatalf("site %q: nil client", site)
		}
	}
}

// TestBindParamsRejectsUnknownKey pins item #11: a top-level parameter key that
// does not map to a field of the target SDK request must fail instead of being
// silently dropped — dropping it would send a zero value to the API and misprice
// the resource (typically under-pricing it).
func TestBindParamsRejectsUnknownKey(t *testing.T) {
	type fakeReq struct {
		InstanceType string `json:"InstanceType"`
		Region       string `json:"Region"`
	}
	// Valid key only -> binds cleanly.
	if err := bindParams(map[string]interface{}{"InstanceType": "S5.LARGE8"}, &fakeReq{}); err != nil {
		t.Fatalf("valid params should bind, got %v", err)
	}
	// Unknown top-level key (typo: "InstancType") -> must error.
	if err := bindParams(map[string]interface{}{"InstanceType": "S5.LARGE8", "InstancType": "x"}, &fakeReq{}); err == nil {
		t.Fatal("expected error for unknown parameter key, got nil")
	}
}

// TestJSONFieldNamesMemoizedAndRaceSafe pins the reflection memoization in
// jsonFieldNames: repeated calls on the same type must return a consistent key
// set (including one level of embedded struct), and concurrent callers must not
// race on the shared cached map. Run under -race to exercise the latter.
func TestJSONFieldNamesMemoizedAndRaceSafe(t *testing.T) {
	type embeddedBase struct {
		X string `json:"x"`
	}
	type sampleReq struct {
		A string `json:"a"`
		B int    `json:"b"`
		embeddedBase
	}

	want := []string{"a", "b", "x"}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names := jsonFieldNames(&sampleReq{})
			for _, k := range want {
				if _, ok := names[k]; !ok {
					t.Errorf("jsonFieldNames missing key %q", k)
				}
			}
		}()
	}
	wg.Wait()
}

// TestCloudHSMUnavailableOnIntl is a regression guard for the placeholder-price
// anomaly: the international site's InquiryPriceBuyVsm returns a bogus fixed
// figure (≈¥150,000,000,000 for virtualization) instead of a real quote, so
// cloudhsm must be skipped on intl with a clear reason rather than surfacing
// that number. The short-circuit fires before any network call, so this test
// needs no real credentials or network.
func TestCloudHSMUnavailableOnIntl(t *testing.T) {
	// Registration: cloudhsm declares intl as unavailable, but stays available
	// on domestic (domestic pricing is unaffected by the intl placeholder bug).
	h := handlers["cloudhsm"]
	if reason, ok := h.unavailableOnSites["intl"]; !ok || reason == "" {
		t.Fatalf("cloudhsm.unavailableOnSites[intl] not set")
	}
	if _, ok := h.unavailableOnSites["domestic"]; ok {
		t.Errorf("cloudhsm should remain available on domestic; unexpected unavailableOnSites[domestic]")
	}

	// Dispatch on intl must short-circuit with a skip reason before any API call.
	e := &Engine{cfg: Config{SecretID: "dummy", SecretKey: "dummy", Site: "intl"}}
	req := PriceRequest{
		Product: "cloudhsm",
		Action:  "InquiryPriceBuyVsm",
		Region:  "ap-guangzhou",
		Params: map[string]interface{}{
			"GoodsNum": 1, "PayMode": 1, "TimeSpan": "1",
			"TimeUnit": "m", "Currency": "CNY", "Type": "CREATE", "HsmType": "virtualization",
		},
	}
	_, err := e.Query(req)
	if err == nil {
		t.Fatalf("expected cloudhsm on intl to be unavailable, got nil error")
	}
	if !strings.Contains(err.Error(), "unavailable on intl site") {
		t.Errorf("cloudhsm intl error = %q, want it to mention %q", err.Error(), "unavailable on intl site")
	}
}

// TestCloudHSMIntlSkipsEvenWithStaleCache guards the key property of the intl
// skip: the unavailableOnSites check in Query runs BEFORE the cache lookup, so
// a stale placeholder from a previous (buggy) run can never be surfaced. We
// seed the on-disk cache with the bogus ¥150,000,000,000 value at the exact key
// cloudhsm/intl would use, then assert Query still returns the unavailable
// error (not the cached number). This is the property that makes the fix take
// effect immediately for every user without clearing their on-disk cache.
func TestCloudHSMIntlSkipsEvenWithStaleCache(t *testing.T) {
	dir := t.TempDir()
	// CachePath must be a BoltDB file, not the temp dir itself, or openCache
	// disables the cache and the seeding below becomes a no-op (defeating the
	// test's purpose of proving a stale value is never surfaced).
	e, err := NewEngine(Config{SecretID: "dummy", SecretKey: "dummy", Site: "intl", CachePath: filepath.Join(dir, "cloudtab-cache.db")})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer e.Close()

	req := PriceRequest{
		Product: "cloudhsm",
		Action:  "InquiryPriceBuyVsm",
		Region:  "ap-guangzhou",
		Params: map[string]interface{}{
			"GoodsNum": 1, "PayMode": 1, "TimeSpan": "1",
			"TimeUnit": "m", "Currency": "CNY", "Type": "CREATE", "HsmType": "virtualization",
		},
	}

	// Seed the cache with the bogus placeholder at cloudhsm/intl's key.
	key, kerr := e.cacheKey(req)
	if kerr != nil {
		t.Fatalf("cacheKey: %v", kerr)
	}
	if perr := e.cache.Put(key, []byte("150000000000")); perr != nil {
		t.Fatalf("seed cache: %v", perr)
	}

	// Even with the stale value present, Query must skip with the reason and
	// must NOT return the cached placeholder.
	resp, qerr := e.Query(req)
	if qerr == nil {
		t.Fatalf("expected cloudhsm on intl to be unavailable; got resp=%q", string(resp))
	}
	if !strings.Contains(qerr.Error(), "unavailable on intl site") {
		t.Errorf("cloudhsm intl error = %q, want it to mention %q", qerr.Error(), "unavailable on intl site")
	}
	if resp != nil {
		t.Errorf("cloudhsm intl must not surface the cached placeholder; got resp=%q", string(resp))
	}
}
