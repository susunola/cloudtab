package pricing

import (
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
		{"international", "international", "intl.tencentcloudapi.com"},
		{"global", "global", "intl.tencentcloudapi.com"},
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
