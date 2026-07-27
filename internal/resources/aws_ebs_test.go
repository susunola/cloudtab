package resources

import (
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
)

func TestAWSEBSVolumeExtract(t *testing.T) {
	r := parser.PlannedResource{
		Address: "aws_ebs_volume.data",
		Type:    "aws_ebs_volume",
		Region:  "eu-central-1",
		After: map[string]interface{}{
			"type": "gp3",
			"size": 100,
		},
	}
	req, err := AWSEBSVolume{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if req.Provider != "aws" || req.Product != "AmazonEC2" {
		t.Fatalf("route = %s/%s, want aws/AmazonEC2", req.Provider, req.Product)
	}
	if got := filterValue(req, "volumeApiName"); got != "gp3" {
		t.Fatalf("volumeApiName = %q, want gp3", got)
	}
	if got := filterValue(req, "location"); got != "EU (Frankfurt)" {
		t.Fatalf("location = %q, want EU (Frankfurt)", got)
	}
	if got := filterValue(req, "productFamily"); got != "Storage" {
		t.Fatalf("productFamily = %q, want Storage", got)
	}
	if got := awsQuantity(req); got != 100 {
		t.Fatalf("stashed Quantity = %d, want 100", got)
	}
}

func TestAWSEBSVolumeExtractDefaultType(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "aws_ebs_volume",
		Region: "us-east-1",
		After: map[string]interface{}{
			"size": 8,
		},
	}
	req, err := AWSEBSVolume{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got := filterValue(req, "volumeApiName"); got != "gp2" {
		t.Fatalf("default volumeApiName = %q, want gp2", got)
	}
}

func TestAWSEBSVolumeExtractZeroSize(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "aws_ebs_volume",
		Region: "us-east-1",
		After:  map[string]interface{}{"type": "gp3"},
	}
	if _, err := (AWSEBSVolume{}).Extract(r); err == nil {
		t.Fatal("expected error for zero/missing size, got nil")
	}
}

func TestAWSEBSVolumeParse(t *testing.T) {
	r := parser.PlannedResource{
		Type:   "aws_ebs_volume",
		Region: "us-east-1",
		After:  map[string]interface{}{"type": "gp3", "size": 100},
	}
	req, err := AWSEBSVolume{}.Extract(r)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	// $0.08 per GB-month * 100 GB = $8.00/month.
	raw := []byte(`[
		{
			"product": {"attributes": {"volumeApiName": "gp3", "usagetype": "EBS:VolumeUsage.gp3"}},
			"terms": {"OnDemand": {"X": {"priceDimensions": {"X.1": {
				"unit": "GB-Mo",
				"pricePerUnit": {"USD": "0.0800000000"}
			}}}}}
		}
	]`)
	comps, err := AWSEBSVolume{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	c := comps[0]
	if c.Currency != "USD" {
		t.Fatalf("Currency = %q, want USD", c.Currency)
	}
	if !almostEqAWS(c.MonthlyCost, 8.0) {
		t.Fatalf("MonthlyCost = %v, want 8.0", c.MonthlyCost)
	}
	if c.HourlyCost != 0 {
		t.Fatalf("HourlyCost = %v, want 0 (storage has no hourly line)", c.HourlyCost)
	}
	if c.Name != "EBS gp3 (100 GB)" {
		t.Fatalf("Name = %q, want EBS gp3 (100 GB)", c.Name)
	}
	if c.Unit != "GB-MONTH" {
		t.Fatalf("Unit = %q, want GB-MONTH", c.Unit)
	}
}

// TestAWSEBSVolumeParsePinsStorageSKU guards against EBS picking the wrong
// dimension. The Price List can return an IOPS/throughput line BEFORE the
// storage line; Parse must pin the storage usagetype ("EBS:VolumeUsage") so a
// gp3 volume is priced off storage only, never the per-IOPS rate.
func TestAWSEBSVolumeParsePinsStorageSKU(t *testing.T) {
	// Two SKUs, IOPS-first: the naive "first positive price" would pick 0.005
	// and multiply by size. We must instead pick the storage line (0.08).
	raw := []byte(`[
		{
			"product": {"attributes": {"usagetype": "EBS:VolumeIOPS.piops"}},
			"terms": {"OnDemand": {"A": {"priceDimensions": {"A.1": {
				"unit": "IOPS-Mo",
				"pricePerUnit": {"USD": "0.005"}
			}}}}}
		},
		{
			"product": {"attributes": {"usagetype": "EBS:VolumeUsage.gp3"}},
			"terms": {"OnDemand": {"B": {"priceDimensions": {"B.1": {
				"unit": "GB-Mo",
				"pricePerUnit": {"USD": "0.08"}
			}}}}}
		}
	]`)

	// Stash the provisioned size the way Extract does.
	req := awsPriceRequest("AmazonEC2", "us-east-1",
		awsFilter("volumeApiName", "gp3"),
		awsFilter("productFamily", "Storage"),
	)
	req.Params["Quantity"] = int64(100)

	comps, err := AWSEBSVolume{}.Parse(req, raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("components = %d, want 1", len(comps))
	}
	if !almostEqAWS(comps[0].MonthlyCost, 8.0) {
		t.Fatalf("MonthlyCost = %v, want 8.0 (0.08 * 100 GB)", comps[0].MonthlyCost)
	}
}
