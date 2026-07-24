package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
)

// writeManyUnsupportedPlan writes a plan JSON containing n resources of a type
// no mapper handles, so every resource is classified as "skipped" (never
// reaching the pricing engine / network). This lets us exercise the
// priceReport worker+collector pipeline at high fan-out deterministically.
func writeManyUnsupportedPlan(t *testing.T, n int) string {
	t.Helper()
	type change struct {
		Actions []string               `json:"actions"`
		After   map[string]interface{} `json:"after"`
	}
	type rc struct {
		Address string `json:"address"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		Change  change `json:"change"`
	}
	doc := struct {
		FormatVersion   string `json:"format_version"`
		ResourceChanges []rc   `json:"resource_changes"`
	}{FormatVersion: "1.2"}
	for i := 0; i < n; i++ {
		doc.ResourceChanges = append(doc.ResourceChanges, rc{
			Address: "unsupported_thing.r" + itoa(i),
			Type:    "unsupported_thing",
			Name:    "r" + itoa(i),
			Change:  change{Actions: []string{"create"}, After: map[string]interface{}{}},
		})
	}
	blob, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return path
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

// TestPriceReportDrainsManyResults is the regression guard for the results
// channel deadlock: with far more resources than workers, the worker->collector
// pipeline must complete rather than hang. An earlier design buffered errors in
// a channel sized to the worker count and only drained it after wg.Wait(),
// which deadlocked once more than `concurrency` resources produced a result
// while every worker was blocked writing. The dedicated concurrent collector
// removes that. We use unsupported types so all results are "skips" and no
// network/credentials are needed.
func TestPriceReportDrainsManyResults(t *testing.T) {
	os.Setenv("TENCENTCLOUD_SECRET_ID", "id")
	os.Setenv("TENCENTCLOUD_SECRET_KEY", "key")
	t.Cleanup(func() {
		os.Unsetenv("TENCENTCLOUD_SECRET_ID")
		os.Unsetenv("TENCENTCLOUD_SECRET_KEY")
	})

	engine, err := pricing.NewEngine(pricing.Config{
		SecretID: "id", SecretKey: "key", Region: "ap-guangzhou", NoCache: true,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	const n = 200 // >> defaultConcurrency (8)
	path := writeManyUnsupportedPlan(t, n)

	// Small concurrency amplifies the fan-in pressure the old design deadlocked
	// under. The test harness's own timeout catches a hang.
	rep, err := priceReport(engine, path, parser.UsageOverrides{}, 2, false)
	if err != nil {
		t.Fatalf("priceReport error: %v", err)
	}
	if len(rep.Skipped) != n {
		t.Fatalf("skipped = %d, want %d", len(rep.Skipped), n)
	}
	if len(rep.Resources) != 0 {
		t.Fatalf("priced = %d, want 0", len(rep.Resources))
	}
}

// TestPriceReportSortsSkipped verifies the concurrently-collected results are
// sorted by address before rendering, so repeated runs of the same plan
// produce deterministic table/JSON output (CI diffs stay quiet).
func TestPriceReportSortsSkipped(t *testing.T) {
	engine, err := pricing.NewEngine(pricing.Config{
		SecretID: "id", SecretKey: "key", Region: "ap-guangzhou", NoCache: true,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	const n = 50 // > worker count, so completion order would otherwise vary
	path := writeManyUnsupportedPlan(t, n)
	rep, err := priceReport(engine, path, parser.UsageOverrides{}, 8, false)
	if err != nil {
		t.Fatalf("priceReport error: %v", err)
	}
	if len(rep.Skipped) != n {
		t.Fatalf("skipped = %d, want %d", len(rep.Skipped), n)
	}
	if !sort.SliceIsSorted(rep.Skipped, func(i, j int) bool {
		return rep.Skipped[i].Address < rep.Skipped[j].Address
	}) {
		t.Fatalf("skipped not sorted by address")
	}
}

// TestPriceReportConcurrencyFloor verifies a zero/negative concurrency is
// clamped to at least one worker (so the pipeline still makes progress) rather
// than spinning up no workers and hanging.
func TestPriceReportConcurrencyFloor(t *testing.T) {
	engine, err := pricing.NewEngine(pricing.Config{
		SecretID: "id", SecretKey: "key", Region: "ap-guangzhou", NoCache: true,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	path := writeManyUnsupportedPlan(t, 5)
	rep, err := priceReport(engine, path, parser.UsageOverrides{}, 0, false) // clamped to 1
	if err != nil {
		t.Fatalf("priceReport error: %v", err)
	}
	if len(rep.Skipped) != 5 {
		t.Fatalf("skipped = %d, want 5", len(rep.Skipped))
	}
}
