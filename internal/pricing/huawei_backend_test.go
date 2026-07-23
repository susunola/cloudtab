package pricing

import (
	"testing"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bssintl/v2/model"
)

// fakeHuaweiBSS captures the ListOnDemandResourceRatings request so tests can
// assert what the backend forwards/injects. The mapper-output shape itself is
// locked separately in the resources package; here we verify the BACKEND
// correctly forwards usage_factor and injects project_id (code review #1/#2).
type fakeHuaweiBSS struct {
	lastReq *model.ListOnDemandResourceRatingsRequest
}

func (f *fakeHuaweiBSS) ListOnDemandResourceRatings(req *model.ListOnDemandResourceRatingsRequest) (*model.ListOnDemandResourceRatingsResponse, error) {
	f.lastReq = req
	return &model.ListOnDemandResourceRatingsResponse{}, nil
}

// representativeECSBody mirrors what HuaweiECS.Extract produces (verified in
// the resources package). Kept inline here so this test does not import the
// resources package (which imports pricing) and thus avoids an import cycle.
func representativeECSBody() map[string]interface{} {
	return map[string]interface{}{
		"product_infos": []map[string]interface{}{
			{
				"id":                 "1",
				"cloud_service_type": "hws.service.type.ec2",
				"resource_type":      "hws.resource.type.vm",
				"resource_spec":      "s3.large.2",
				"region":             "cn-north-4",
				"usage_factor":       "Duration",
				"usage_value":        1,
				"usage_measure_id":   1,
				"subscription_num":   1,
			},
		},
	}
}

func TestHuaweiBackendForwardsUsageFactorAndInjectsProjectID(t *testing.T) {
	const projectID = "00000000-0000-0000-0000-000000000000"
	req := PriceRequest{Provider: "huawei", Product: "ecs", Region: "cn-north-4", Params: representativeECSBody()}

	fake := &fakeHuaweiBSS{}
	b := &huaweiBackend{client: fake, projectID: projectID}
	if _, err := b.query(req); err != nil {
		t.Fatalf("query() error = %v", err)
	}
	body := fake.lastReq.Body
	if body.ProjectId != projectID {
		t.Errorf("ProjectId = %q, want injected %q", body.ProjectId, projectID)
	}
	if len(body.ProductInfos) == 0 {
		t.Fatalf("no product_infos")
	}
	if body.ProductInfos[0].UsageFactor != "Duration" {
		t.Errorf("UsageFactor = %q, want Duration", body.ProductInfos[0].UsageFactor)
	}
}

func TestHuaweiBackendNoProjectIDWhenUnset(t *testing.T) {
	req := PriceRequest{Provider: "huawei", Product: "ecs", Params: representativeECSBody()}

	fake := &fakeHuaweiBSS{}
	b := &huaweiBackend{client: fake}
	if _, err := b.query(req); err != nil {
		t.Fatalf("query() error = %v", err)
	}
	if fake.lastReq.Body.ProjectId != "" {
		t.Errorf("ProjectId = %q, want empty when backend has no projectID", fake.lastReq.Body.ProjectId)
	}
}
