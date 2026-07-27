package pricing

import (
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/bssopenapi"
)

// TestBuildAlibabaRequest verifies the pure request-builder maps neutral
// Params into the typed BSS request (and defaults SubscriptionType) without a
// client.
func TestBuildAlibabaRequest(t *testing.T) {
	in, err := buildAlibabaRequest(PriceRequest{
		Provider: "alibaba",
		Product:  "ecs",
		Region:   "cn-hangzhou",
		Params: map[string]interface{}{
			"ModuleList": []map[string]string{
				{"ModuleCode": "InstanceType", "PriceType": "Hour", "Config": "ecs.s6.large.2:linux"},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildAlibabaRequest error = %v", err)
	}
	if in.ProductCode != "ecs" {
		t.Errorf("ProductCode = %q, want ecs", in.ProductCode)
	}
	if in.Region != "cn-hangzhou" {
		t.Errorf("Region = %q, want cn-hangzhou", in.Region)
	}
	if in.SubscriptionType != "PayAsYouGo" {
		t.Errorf("SubscriptionType = %q, want default PayAsYouGo", in.SubscriptionType)
	}
	if in.ModuleList == nil || len(*in.ModuleList) != 1 {
		t.Fatalf("ModuleList = %v, want 1 module", in.ModuleList)
	}
	if (*in.ModuleList)[0].ModuleCode != "InstanceType" || (*in.ModuleList)[0].Config != "ecs.s6.large.2:linux" {
		t.Errorf("ModuleList[0] = %+v, want InstanceType/ecs.s6.large.2:linux", (*in.ModuleList)[0])
	}
}

// TestBuildAlibabaRequestMissingProduct confirms an empty ProductCode errors.
func TestBuildAlibabaRequestMissingProduct(t *testing.T) {
	if _, err := buildAlibabaRequest(PriceRequest{Provider: "alibaba"}); err == nil {
		t.Fatal("expected error for missing Product, got nil")
	}
}

// fakeAlibabaBSS captures the GetPayAsYouGoPrice request so tests can assert
// what the backend forwards. The mapper-output shape is locked in the resources
// package; here we verify the backend forwards the ModuleList correctly and
// that the dead "Quantity" field (mappers no longer send) is irrelevant.
type fakeAlibabaBSS struct {
	lastReq *bssopenapi.GetPayAsYouGoPriceRequest
}

func (f *fakeAlibabaBSS) GetPayAsYouGoPrice(req *bssopenapi.GetPayAsYouGoPriceRequest) (*bssopenapi.GetPayAsYouGoPriceResponse, error) {
	f.lastReq = req
	return &bssopenapi.GetPayAsYouGoPriceResponse{}, nil
}

func TestAlibabaBackendForwardsModuleList(t *testing.T) {
	req := PriceRequest{
		Provider: "alibaba",
		Product:  "ecs",
		Region:   "cn-hangzhou",
		Params: map[string]interface{}{
			"SubscriptionType": "PayAsYouGo",
			"ModuleList": []map[string]string{
				{"ModuleCode": "InstanceType", "PriceType": "Hour", "Config": "ecs.s6.large.2:linux"},
			},
		},
	}

	fake := &fakeAlibabaBSS{}
	b := &alibabaBackend{client: fake}
	if _, err := b.query(req); err != nil {
		t.Fatalf("query() error = %v", err)
	}
	in := fake.lastReq
	if in == nil {
		t.Fatalf("backend captured no request")
	}
	if in.SubscriptionType != "PayAsYouGo" {
		t.Errorf("SubscriptionType = %q, want PayAsYouGo", in.SubscriptionType)
	}
	if in.ProductCode != "ecs" {
		t.Errorf("ProductCode = %q, want ecs", in.ProductCode)
	}
	if in.ModuleList == nil || len(*in.ModuleList) != 1 {
		t.Fatalf("ModuleList = %v, want 1 module", in.ModuleList)
	}
	if (*in.ModuleList)[0].ModuleCode != "InstanceType" {
		t.Errorf("ModuleCode = %q, want InstanceType", (*in.ModuleList)[0].ModuleCode)
	}
}
