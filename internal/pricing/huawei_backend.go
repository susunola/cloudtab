// Package pricing — Huawei Cloud BSS pricing backend.
//
// Huawei Cloud exposes pricing through the BSS ListOnDemandResourceRatings API,
// a unified endpoint that prices all pay-per-use (on-demand) resources by
// product spec, region, and usage factor. This backend handles the single API
// call; each mapper (in internal/resources/huawei_*.go) builds the appropriate
// product_infos payload and parses the response.
//
// Contract with Huawei mappers:
//   - req.Product  = informational label (e.g. "ecs", "rds", "dcs").
//   - req.Region   = the Huawei Cloud region (e.g. "cn-north-4", "ap-singapore").
//   - req.Params   = the RateOnDemandReq body fields:
//     "product_infos":     []DemandProductInfo
//     where each DemandProductInfo has: id, cloud_service_type, resource_type,
//     resource_spec, region, usage_factor, usage_value, usage_measure_id,
//     subscription_num. The project_id (a UUID, NOT the region) is injected by
//     the backend from Config.HuaweiProjectID / HUAWEI_PROJECT_ID — mappers
//     must NOT set it.
//
// Site selection: Huawei Cloud runs two independent BSS sites with separate
// account systems — the International site (bss-intl.myhuaweicloud.com, SDK
// package bssintl/v2) and the Chinese-mainland site (bss.myhuaweicloud.com, SDK
// package bss/v2). A credential is registered on exactly ONE of them, so it is
// selected explicitly via Config.HuaweiSite, NOT derived from the region. The
// two SDK request/response types differ, so the cn client is wrapped in an
// adapter (cnHuaweiBSSAdapter) that implements the intl-shaped huaweiBSSAPI by
// JSON round-tripping the body; query() stays type-agnostic.
package pricing

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	bss "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2"
	bssmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
	bssregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/region"
	bssintl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bssintl/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bssintl/v2/model"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bssintl/v2/region"
)

// huaweiBSSAPI is the subset of the Huawei BSS client we use. It is typed to the
// International (bssintl) request/response; the Chinese-mainland (bss) client is
// exposed through cnHuaweiBSSAdapter, which satisfies this same interface, so
// query() needs no per-site branching.
type huaweiBSSAPI interface {
	ListOnDemandResourceRatings(request *model.ListOnDemandResourceRatingsRequest) (*model.ListOnDemandResourceRatingsResponse, error)
}

// huaweiBackend implements backend using the Huawei BSS ListOnDemandResourceRatings API.
type huaweiBackend struct {
	client    huaweiBSSAPI
	projectID string // injected into RateOnDemandReq.ProjectId
}

// cnHuaweiBSSAdapter adapts the Chinese-mainland (cn) BSS client to the
// intl-shaped huaweiBSSAPI. The cn and intl request/response types are
// identical in JSON shape (only the Go package differs), so we round-trip the
// body through JSON to convert between them.
type cnHuaweiBSSAdapter struct {
	client *bss.BssClient
}

func (a *cnHuaweiBSSAdapter) ListOnDemandResourceRatings(in *model.ListOnDemandResourceRatingsRequest) (*model.ListOnDemandResourceRatingsResponse, error) {
	bodyBytes, err := json.Marshal(in.Body)
	if err != nil {
		return nil, fmt.Errorf("huawei cn: marshal request: %w", err)
	}
	var cnBody bssmodel.RateOnDemandReq
	if err := json.Unmarshal(bodyBytes, &cnBody); err != nil {
		return nil, fmt.Errorf("huawei cn: unmarshal request: %w", err)
	}
	cnIn := &bssmodel.ListOnDemandResourceRatingsRequest{Body: &cnBody}
	cnOut, err := a.client.ListOnDemandResourceRatings(cnIn)
	if err != nil {
		return nil, fmt.Errorf("huawei ListOnDemandResourceRatings %s: %w", "", err)
	}
	outBytes, err := json.Marshal(cnOut)
	if err != nil {
		return nil, fmt.Errorf("huawei cn: marshal response: %w", err)
	}
	var out model.ListOnDemandResourceRatingsResponse
	if err := json.Unmarshal(outBytes, &out); err != nil {
		return nil, fmt.Errorf("huawei cn: unmarshal response: %w", err)
	}
	return &out, nil
}

// newHuaweiBackend builds the Huawei BSS backend. Credentials are resolved
// from Config.HuaweiAccessKeyID / Config.HuaweiSecretAccessKey, falling back
// to environment variables HUAWEI_ACCESS_KEY_ID / HUAWEI_SECRET_ACCESS_KEY.
//
// The BSS service uses Global-level authentication (not project-level), and
// the endpoint is resolved by the SDK per region. The site (International vs
// Chinese-mainland) is chosen by Config.HuaweiSite and selects between the
// bssintl and bss SDK packages — a latent bug previously hard-coded the
// international client even for China-mainland credentials.
func newHuaweiBackend(cfg Config) (backend, error) {
	ak := cfg.HuaweiAccessKeyID
	sk := cfg.HuaweiSecretAccessKey
	if ak == "" {
		ak = os.Getenv("HUAWEI_ACCESS_KEY_ID")
	}
	if sk == "" {
		sk = os.Getenv("HUAWEI_SECRET_ACCESS_KEY")
	}
	if ak == "" || sk == "" {
		return nil, fmt.Errorf("huawei: missing access key (set HUAWEI_ACCESS_KEY_ID / HUAWEI_SECRET_ACCESS_KEY or Config.HuaweiAccessKeyID / HuaweiSecretAccessKey)")
	}

	auth := global.NewCredentialsBuilder().
		WithAk(ak).
		WithSk(sk).
		Build()

	httpConfig := config.DefaultHttpConfig().WithTimeout(cfg.requestTimeout())

	// Choose the SDK package by site: bssintl for International (default),
	// bss for the Chinese-mainland site. This fixes the latent misroute where
	// a China-mainland credential was wrongly sent to
	// bss-intl.myhuaweicloud.com.
	//
	// The two BSS services expose DIFFERENT endpoint regions: the cn service
	// only resolves cn-north-1, while the intl service resolves ap-southeast-1
	// (bss-intl.myhuaweicloud.com) and eu-west-101. We therefore pick a
	// site-appropriate default and validate any caller override with
	// SafeValueOf so a bad region id returns an error instead of panicking.
	useCN := strings.TrimSpace(cfg.HuaweiSite) != "" && normalizeSite(cfg.HuaweiSite) == "domestic"

	defaultRegion := "ap-southeast-1"
	if useCN {
		defaultRegion = "cn-north-1"
	}
	endpointRegion := cfg.HuaweiBSSEndpointRegion
	if endpointRegion == "" {
		endpointRegion = defaultRegion
	}

	var client huaweiBSSAPI
	if useCN {
		r, err := bssregion.SafeValueOf(endpointRegion)
		if err != nil {
			// Fall back to the only cn-supported BSS region rather than panic.
			r, _ = bssregion.SafeValueOf("cn-north-1")
		}
		cnClient := bss.NewBssClient(
			bss.BssClientBuilder().
				WithRegion(r).
				WithCredential(auth).
				// Bound each HTTP round-trip so a stalled Huawei BSS call cannot
				// hang the whole run (code review #4). --timeout now applies here
				// too, matching the Tencent and AWS backends.
				WithHttpConfig(httpConfig).
				Build(),
		)
		client = &cnHuaweiBSSAdapter{client: cnClient}
	} else {
		r, err := region.SafeValueOf(endpointRegion)
		if err != nil {
			// Fall back to the intl default endpoint rather than panic.
			r, _ = region.SafeValueOf("ap-southeast-1")
		}
		client = bssintl.NewBssintlClient(
			bssintl.BssintlClientBuilder().
				WithRegion(r).
				WithCredential(auth).
				// Bound each HTTP round-trip so a stalled Huawei BSS call cannot
				// hang the whole run (code review #4). --timeout now applies here
				// too, matching the Tencent and AWS backends.
				WithHttpConfig(httpConfig).
				Build(),
		)
	}

	// ProjectId (a UUID, NOT the region) is injected by the backend from
	// Config.HuaweiProjectID / HUAWEI_PROJECT_ID. Mappers must not set it.
	return &huaweiBackend{client: client, projectID: cfg.HuaweiProjectID}, nil
}

// query runs a single ListOnDemandResourceRatings call.
//
// The mapper's Extract() populates req.Params with the RateOnDemandReq fields
// (project_id + product_infos). This method builds the typed SDK request,
// executes it (via the site-appropriate client, hidden behind huaweiBSSAPI),
// and returns the raw response JSON.
func (b *huaweiBackend) query(req PriceRequest) ([]byte, error) {
	// Marshal params to JSON, then unmarshal into RateOnDemandReq.
	bodyBytes, err := json.Marshal(req.Params)
	if err != nil {
		return nil, fmt.Errorf("huawei: marshal params: %w", err)
	}
	var body model.RateOnDemandReq
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return nil, fmt.Errorf("huawei: unmarshal params: %w", err)
	}

	// Inject the project id (UUID) that mappers must not set. It is the
	// RateOnDemandReq.ProjectId, distinct from the per-product region. When
	// unset we leave the (empty) value so the API bills under the credential's
	// default project.
	if b.projectID != "" {
		body.ProjectId = b.projectID
	}

	in := &model.ListOnDemandResourceRatingsRequest{Body: &body}
	out, err := b.client.ListOnDemandResourceRatings(in)
	if err != nil {
		return nil, fmt.Errorf("huawei ListOnDemandResourceRatings %s: %w", req.Product, err)
	}
	resp, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("huawei: marshal response: %w", err)
	}
	return resp, nil
}
