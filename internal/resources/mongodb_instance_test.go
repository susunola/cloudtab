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

func TestMongoDBExtractNormalizesMachineCode(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wantMC string
	}{
		{"legacy HIO", "HIO", "HIO10G"},
		{"current HIO10G", "HIO10G", "HIO10G"},
		{"cloud disk", "HCD", "HCD"},
		{"empty defaults to GE.LD.T1", "", "GE.LD.T1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after := map[string]interface{}{
				"available_zone": "ap-guangzhou-3",
				"memory":         4,
				"volume":         100,
				"charge_type":    "POSTPAID_BY_HOUR",
			}
			if tc.input != "" {
				after["machine_type"] = tc.input
			}
			req, err := MongoDBInstance{}.Extract(parser.PlannedResource{
				Type:   "tencentcloud_mongodb_instance",
				Region: "ap-guangzhou",
				After:  after,
			})
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			if req.Params["MachineCode"] != tc.wantMC {
				t.Errorf("MachineCode = %v, want %v", req.Params["MachineCode"], tc.wantMC)
			}
		})
	}
}

func TestMongoDBExtractDefaultsMongoVersion(t *testing.T) {
	// When engine_version is absent, MongoVersion should default to MONGO_40_WT.
	req, err := MongoDBInstance{}.Extract(parser.PlannedResource{
		Type:   "tencentcloud_mongodb_instance",
		Region: "ap-guangzhou",
		After: map[string]interface{}{
			"available_zone": "ap-guangzhou-3",
			"memory":         4,
			"volume":         100,
			"charge_type":    "PREPAID",
		},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Params["MongoVersion"] != "MONGO_40_WT" {
		t.Errorf("MongoVersion = %v, want MONGO_40_WT", req.Params["MongoVersion"])
	}
}
