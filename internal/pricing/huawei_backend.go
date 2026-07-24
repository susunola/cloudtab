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
package pricing

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	bssintl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bssintl/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bssintl/v2/model"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bssintl/v2/region"
)

// huaweiBSSAPI is the subset of the Huawei BSS international client we use.
type huaweiBSSAPI interface {
	ListOnDemandResourceRatings(request *model.ListOnDemandResourceRatingsRequest) (*model.ListOnDemandResourceRatingsResponse, error)
}

// huaweiBackend implements backend using the Huawei BSS ListOnDemandResourceRatings API.
type huaweiBackend struct {
	client    huaweiBSSAPI
	projectID string // injected into RateOnDemandReq.ProjectId
}

// newHuaweiBackend builds the Huawei BSS backend. Credentials are resolved
// from Config.HuaweiAccessKeyID / Config.HuaweiSecretAccessKey, falling back
// to environment variables HUAWEI_ACCESS_KEY_ID / HUAWEI_SECRET_ACCESS_KEY.
//
// The BSS service uses Global-level authentication (not project-level), and
// the endpoint is resolved by the SDK per region.
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

	client := bssintl.NewBssintlClient(
		bssintl.BssintlClientBuilder().
			WithRegion(region.ValueOf("cn-north-4")).
			WithCredential(auth).
			// Bound each HTTP round-trip so a stalled Huawei BSS call cannot
			// hang the whole run (code review #4). --timeout now applies here
			// too, matching the Tencent and AWS backends.
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(cfg.requestTimeout())).
			Build(),
	)
	// ProjectId (a UUID, NOT the region) is injected by the backend from
	// Config.HuaweiProjectID / HUAWEI_PROJECT_ID. Mappers must not set it.
	return &huaweiBackend{client: client, projectID: cfg.HuaweiProjectID}, nil
}

// query runs a single ListOnDemandResourceRatings call.
//
// The mapper's Extract() populates req.Params with the RateOnDemandReq fields
// (project_id + product_infos). This method builds the typed SDK request,
// executes it, and returns the raw response JSON.
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
