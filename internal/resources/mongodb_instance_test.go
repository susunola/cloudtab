package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

func TestMongoDBExtractNormalizesPostpaid(t *testing.T) {
	// Terraform charge_type "POSTPAID" must map to the pricing API enum value
	// "POSTPAID_BY_HOUR"; sending the raw "POSTPAID" makes the API reject the
	// request with an enum error.
	req, err := MongoDBInstance{}.Extract(parser.PlannedResource{
		Type:   "tencentcloud_mongodb_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"memory":         4,
			"volume":         100,
			"charge_type":    "POSTPAID",
		},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Params["InstanceChargeType"] != "POSTPAID_BY_HOUR" {
		t.Fatalf("InstanceChargeType = %v, want POSTPAID_BY_HOUR", req.Params["InstanceChargeType"])
	}
}
