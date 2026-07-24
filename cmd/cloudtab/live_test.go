//go:build live

// Package main smoke tests that require REAL cloud credentials. These are
// intentionally excluded from the normal suite (build tag `live`) so CI never
// needs secrets or network. Run them manually to validate the live pricing
// backends end-to-end:
//
//	go test -tags live -run TestLivePricingSmoke ./cmd/cloudtab/
//
// Set whichever provider credentials you have; the test only prices the
// providers it finds and skips the rest. With no credentials at all it skips
// entirely. This is the home for the "needs real credentials" items from the
// coverage gap analysis.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

type liveResource struct {
	Address string     `json:"address"`
	Type    string     `json:"type"`
	Name    string     `json:"name"`
	Change  liveChange `json:"change"`
}

type liveChange struct {
	Actions []string               `json:"actions"`
	After   map[string]interface{} `json:"after"`
}

// TestLivePricingSmoke drives the real priceReport pipeline (mapper -> engine ->
// provider API -> Parse) for every provider with credentials present in the
// environment, and asserts the run completes with at least one priced resource.
// It is a manual guard against silent backend drift (a changed API shape or
// expired credential surfaces as a priced==0 / all-skipped result here).
func TestLivePricingSmoke(t *testing.T) {
	cfg := pricing.Config{NoCache: true}

	// Tencent Cloud (domestic).
	if id := os.Getenv("TENCENTCLOUD_SECRET_ID"); id != "" {
		cfg.SecretID = id
		cfg.SecretKey = os.Getenv("TENCENTCLOUD_SECRET_KEY")
		cfg.Region = firstNonEmpty(os.Getenv("TENCENTCLOUD_REGION"), "ap-guangzhou")
	}
	// AWS (falls back to the SDK default credential chain when unset).
	if id := os.Getenv("AWS_ACCESS_KEY_ID"); id != "" {
		cfg.AWSAccessKeyID = id
		cfg.AWSSecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
		cfg.AWSSessionToken = os.Getenv("AWS_SESSION_TOKEN")
	}
	// Alibaba Cloud.
	if id := os.Getenv("ALIBABA_ACCESS_KEY_ID"); id != "" {
		cfg.AlibabaAccessKeyID = id
		cfg.AlibabaAccessKeySecret = os.Getenv("ALIBABA_ACCESS_KEY_SECRET")
	}
	// Huawei Cloud.
	if id := os.Getenv("HUAWEI_ACCESS_KEY_ID"); id != "" {
		cfg.HuaweiAccessKeyID = id
		cfg.HuaweiSecretAccessKey = os.Getenv("HUAWEI_SECRET_ACCESS_KEY")
		cfg.HuaweiProjectID = os.Getenv("HUAWEI_PROJECT_ID")
	}

	// Which providers have usable credentials? Build a plan for each.
	var changes []liveResource
	add := func(addr, typ string, after map[string]interface{}) {
		changes = append(changes, liveResource{
			Address: addr, Type: typ, Name: addr,
			Change: liveChange{Actions: []string{"create"}, After: after},
		})
	}
	if cfg.SecretID != "" {
		add("tencentcloud_instance.live", "tencentcloud_instance", map[string]interface{}{
			"instance_type": "S5.LARGE8", "image_id": "img-xxx", "availability_zone": "ap-guangzhou-6",
			"instance_charge_type": "POSTPAID",
		})
	}
	if cfg.AWSAccessKeyID != "" {
		add("aws_instance.live", "aws_instance", map[string]interface{}{
			"instance_type": "m5.large", "tenancy": "default",
		})
	}
	if cfg.AlibabaAccessKeyID != "" {
		add("alicloud_instance.live", "alicloud_instance", map[string]interface{}{
			"instance_type": "ecs.s6.large.2",
		})
	}
	if cfg.HuaweiAccessKeyID != "" {
		add("huaweicloud_compute_instance.live", "huaweicloud_compute_instance", map[string]interface{}{
			"flavor_id": "s3.large.2",
		})
	}

	if len(changes) == 0 {
		t.Skip("no cloud credentials in env (set TENCENTCLOUD_SECRET_ID/KEY, AWS_*, ALIBABA_*, HUAWEI_* to run live smoke)")
	}

	doc := struct {
		FormatVersion   string         `json:"format_version"`
		ResourceChanges []liveResource `json:"resource_changes"`
	}{FormatVersion: "1.2", ResourceChanges: changes}
	blob, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	path := filepath.Join(t.TempDir(), "live-plan.json")
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	engine, err := pricing.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	rep, err := priceReport(engine, path, parser.UsageOverrides{}, 4, false)
	if err != nil {
		t.Fatalf("priceReport returned error: %v", err)
	}
	if len(rep.Resources) == 0 {
		t.Fatalf("live smoke priced 0 resources; skipped=%d (%+v)", len(rep.Skipped), rep.Skipped)
	}
	// At least one priced component must carry a positive monthly cost.
	priced := 0
	for _, r := range rep.Resources {
		for _, c := range r.Components {
			if c.MonthlyCost > 0 {
				priced++
			}
		}
	}
	if priced == 0 {
		t.Fatalf("live smoke found no resource with MonthlyCost > 0; priced=%d skipped=%d", len(rep.Resources), len(rep.Skipped))
	}
	t.Logf("live smoke OK: %d priced resource(s), %d skipped", len(rep.Resources), len(rep.Skipped))
}

func firstNonEmpty(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}
