//go:build integration

// Package main integration tests exercise the built cloudtab BINARY end to end
// (real cobra wiring, arg parsing, output rendering, skip + diff logic) against
// committed Terraform plan fixtures — not just Go units. They are excluded from
// the normal suite (build tag `integration`) because they compile the binary
// and shell out to it. Run them with:
//
//	go test -tags integration ./cmd/cloudtab/
//
// Most scenarios run WITHOUT any cloud credentials: cloud resources are
// expected to be skipped (API unreachable / unauthenticated), while the
// tencentcloud_eip fixture is a StaticMapper that prices locally and offline,
// giving us a real priced resource to assert on deterministically.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cloudtabBin is the compiled binary, built once in TestMain.
var cloudtabBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cloudtab-itest")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	cloudtabBin = filepath.Join(dir, "cloudtab")
	build := exec.Command("go", "build", "-o", cloudtabBin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build cloudtab failed: %v\n%s\n", err, out)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// run invokes the binary with a credential-free environment so cloud-skip
// assertions are deterministic and we never touch real cloud accounts or the
// user's on-disk cache.
func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(cloudtabBin, args...)
	cmd.Env = cleanEnv()
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return so.String(), se.String(), code
}

// runRaw invokes the binary with the inherited environment (used by the opt-in
// live-priced test that only runs when real credentials are present).
func runRaw(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(cloudtabBin, args...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return so.String(), se.String(), code
}

// fixture resolves a path to a committed plan under <repo>/testdata, regardless
// of the process working directory (the test runs from cmd/cloudtab, but the
// fixtures live at the repo root).
func fixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test source directory")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "testdata", name)
}

// cleanEnv returns the current environment with all cloud credentials and the
// concurrency override stripped, so a run is hermetic.
func cleanEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k := kv[:strings.IndexByte(kv, '=')]
		switch k {
		case "TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY",
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
			"HUAWEI_PROJECT_ID", "TENCENTCLOUD_SITE", "CLOUDTAB_CONCURRENCY":
			continue
		}
		env = append(env, kv)
	}
	return env
}

// jsonReport mirrors output.Report's JSON shape (only the fields we assert).
type jsonReport struct {
	Resources []struct {
		Address    string `json:"address"`
		Type       string `json:"type"`
		Components []struct {
			Name        string  `json:"name"`
			MonthlyCost float64 `json:"monthly_cost"`
			Currency    string  `json:"currency"`
		} `json:"components"`
	} `json:"resources"`
	Skipped []struct {
		Address string `json:"address"`
		Type    string `json:"type"`
		Reason  string `json:"reason"`
	} `json:"skipped"`
}

// jsonDiff mirrors output.DiffReport's JSON shape (only the fields we assert).
type jsonDiff struct {
	Resources []struct {
		Address      string  `json:"address"`
		Kind         string  `json:"kind"`
		AfterMonthly float64 `json:"after_monthly"`
	} `json:"resources"`
	Skipped []struct {
		Address string `json:"address"`
		Reason  string `json:"reason"`
	} `json:"skipped"`
}

func mustJSONReport(t *testing.T, stdout string) jsonReport {
	t.Helper()
	var r jsonReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("stdout is not a valid report JSON: %v\n--- stdout ---\n%s", err, stdout)
	}
	return r
}

func mustJSONDiff(t *testing.T, stdout string) jsonDiff {
	t.Helper()
	var d jsonDiff
	if err := json.Unmarshal([]byte(stdout), &d); err != nil {
		t.Fatalf("stdout is not a valid diff JSON: %v\n--- stdout ---\n%s", err, stdout)
	}
	return d
}

// TestCLIVersion pins the `version` subcommand output.
func TestCLIVersion(t *testing.T) {
	out, _, code := run(t, "version")
	if code != 0 {
		t.Fatalf("version exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "cloudtab") {
		t.Errorf("version output missing binary name: %q", out)
	}
}

// TestBreakdownEIPSkippedWithoutCreds pins the no-credentials behavior for EIP:
// tencentcloud_eip is priced via the vpc InquiryPriceAllocateAddresses API, so
// without credentials the resource is skipped with a reason (never crashes,
// exit 0).
func TestBreakdownEIPSkippedWithoutCreds(t *testing.T) {
	out, _, code := run(t, "breakdown",
		"--path", fixture(t, "static_eip_plan.json"),
		"--format", "json",
		"--cache-dir", t.TempDir(), "--timeout", "3s")
	if code != 0 {
		t.Fatalf("breakdown exit code = %d, want 0", code)
	}
	rep := mustJSONReport(t, out)
	if len(rep.Resources) != 0 {
		t.Fatalf("resources = %d, want 0 (no creds -> EIP skipped)", len(rep.Resources))
	}
	if len(rep.Skipped) != 1 {
		t.Fatalf("skipped = %d, want 1", len(rep.Skipped))
	}
	s := rep.Skipped[0]
	if s.Address != "tencentcloud_eip.web" {
		t.Errorf("skipped address = %q, want tencentcloud_eip.web", s.Address)
	}
	if s.Reason == "" {
		t.Errorf("skipped reason is empty; a skip must always carry a reason")
	}
}

// TestBreakdownTableRendersEIP checks the table renderer emits the EIP address
// even when the resource is skipped (offline, no credentials).
func TestBreakdownTableRendersEIP(t *testing.T) {
	out, _, code := run(t, "breakdown",
		"--path", fixture(t, "static_eip_plan.json"),
		"--format", "table",
		"--cache-dir", t.TempDir(), "--timeout", "3s")
	if code != 0 {
		t.Fatalf("breakdown exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "tencentcloud_eip.web") {
		t.Errorf("table output missing EIP address:\n%s", out)
	}
}

// TestBreakdownSkipsCloudResourceWithoutCreds pins the no-credentials behavior:
// a pure cloud resource (tencentcloud_instance) must be skipped with a reason,
// never crash, and the run must still exit 0.
func TestBreakdownSkipsCloudResourceWithoutCreds(t *testing.T) {
	out, _, code := run(t, "breakdown",
		"--path", fixture(t, "example.plan.json"),
		"--format", "json",
		"--cache-dir", t.TempDir(), "--timeout", "3s")
	if code != 0 {
		t.Fatalf("breakdown exit code = %d, want 0 (failOnError default false)", code)
	}
	rep := mustJSONReport(t, out)
	if len(rep.Resources) != 0 {
		t.Fatalf("resources = %d, want 0 (no creds -> cloud resource skipped)", len(rep.Resources))
	}
	if len(rep.Skipped) != 1 {
		t.Fatalf("skipped = %d, want 1", len(rep.Skipped))
	}
	s := rep.Skipped[0]
	if s.Address != "tencentcloud_instance.web" {
		t.Errorf("skipped address = %q, want tencentcloud_instance.web", s.Address)
	}
	if s.Reason == "" {
		t.Errorf("skipped reason is empty; a skip must always carry a reason")
	}
}

// TestBreakdownBadFormatRejects guards review item #18: --format is validated
// BEFORE the engine is built, so a bad format fails fast (exit 1) without
// touching credentials or network.
func TestBreakdownBadFormatRejects(t *testing.T) {
	_, serr, code := run(t, "breakdown",
		"--path", fixture(t, "static_eip_plan.json"),
		"--format", "bogus")
	if code == 0 {
		t.Fatalf("bad --format should exit non-zero, got 0")
	}
	if !strings.Contains(serr, "unknown format") && !strings.Contains(serr, "format") {
		t.Errorf("error should mention the bad format: %q", serr)
	}
}

// TestBreakdownRejectsExtraArgs guards review item #19: breakdown uses
// cobra.NoArgs, so a stray positional arg is rejected (exit 1).
func TestBreakdownRejectsExtraArgs(t *testing.T) {
	_, _, code := run(t, "breakdown", "stray-positional-arg")
	if code == 0 {
		t.Fatalf("extra positional arg should be rejected, got exit 0")
	}
}

// TestDiffRequiresBeforeAfter pins that diff needs both --before and --after.
func TestDiffRequiresBeforeAfter(t *testing.T) {
	_, _, code := run(t, "diff")
	if code == 0 {
		t.Fatalf("diff without --before/--after should exit non-zero, got 0")
	}
}

// TestDiffBadFormatRejects guards item #18 for the diff command too.
func TestDiffBadFormatRejects(t *testing.T) {
	_, serr, code := run(t, "diff",
		"--before", fixture(t, "example.plan.json"),
		"--after", fixture(t, "static_eip_plan.json"),
		"--format", "bogus")
	if code == 0 {
		t.Fatalf("bad --format should exit non-zero, got 0")
	}
	if !strings.Contains(serr, "unknown format") && !strings.Contains(serr, "format") {
		t.Errorf("error should mention the bad format: %q", serr)
	}
}

// TestDiffAcrossFixtures exercises the real diff computation end to end:
// before = a skipped cloud CVM, after = a skipped EIP. Without credentials both
// resources are skipped; the diff's skipped list contains both addresses and no
// resources are priced.
func TestDiffAcrossFixtures(t *testing.T) {
	out, _, code := run(t, "diff",
		"--before", fixture(t, "example.plan.json"),
		"--after", fixture(t, "static_eip_plan.json"),
		"--format", "json",
		"--cache-dir", t.TempDir(), "--timeout", "3s")
	if code != 0 {
		t.Fatalf("diff exit code = %d, want 0", code)
	}
	d := mustJSONDiff(t, out)
	if len(d.Resources) != 0 {
		t.Fatalf("diff resources = %d, want 0 (no creds -> all skipped)", len(d.Resources))
	}
	if len(d.Skipped) != 2 {
		t.Fatalf("diff skipped = %d, want 2 (CVM + EIP)", len(d.Skipped))
	}
	// Skipped is sorted by address: tencentcloud_eip.web < tencentcloud_instance.web
	if d.Skipped[0].Address != "tencentcloud_eip.web" {
		t.Errorf("diff skipped[0] address = %q, want tencentcloud_eip.web", d.Skipped[0].Address)
	}
	if d.Skipped[1].Address != "tencentcloud_instance.web" {
		t.Errorf("diff skipped[1] address = %q, want tencentcloud_instance.web", d.Skipped[1].Address)
	}
}

// TestBreakdownMissingPlanFile pins the missing-input error path.
func TestBreakdownMissingPlanFile(t *testing.T) {
	_, _, code := run(t, "breakdown",
		"--path", filepath.Join(t.TempDir(), "does-not-exist.json"),
		"--format", "json")
	if code == 0 {
		t.Fatalf("missing plan file should exit non-zero, got 0")
	}
}

// TestBreakdownWithCredsPricing is an opt-in live check: when Tencent Cloud
// credentials are present in the environment it prices a real cloud resource
// and asserts at least one resource is priced (not all skipped). Without
// credentials it is skipped, keeping the suite hermetic by default.
func TestBreakdownWithCredsPricing(t *testing.T) {
	if os.Getenv("TENCENTCLOUD_SECRET_ID") == "" || os.Getenv("TENCENTCLOUD_SECRET_KEY") == "" {
		t.Skip("no Tencent Cloud credentials; set TENCENTCLOUD_SECRET_ID/KEY to run the live priced check")
	}
	out, _, code := runRaw(t, "breakdown",
		"--path", fixture(t, "example.plan.json"),
		"--format", "json",
		"--cache-dir", t.TempDir(), "--timeout", "30s")
	if code != 0 {
		t.Fatalf("breakdown exit code = %d, want 0", code)
	}
	rep := mustJSONReport(t, out)
	if len(rep.Resources) == 0 {
		t.Errorf("with credentials, expected at least one priced resource, got none (all skipped: %+v)", rep.Skipped)
	}
}

func TestBreakdownPolicyFailureRendersJSONAndExitsTwo(t *testing.T) {
	out, serr, code := run(t, "breakdown",
		"--path", fixture(t, "example.plan.json"),
		"--format", "json", "--no-cache", "--timeout", "1s",
		"--fail-on-skipped")
	if code != 2 {
		t.Fatalf("policy failure exit code = %d, want 2; stderr=%s", code, serr)
	}
	var document map[string]interface{}
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatalf("policy-failed stdout is not valid JSON: %v\n%s", err, out)
	}
	policyValue, ok := document["policy"].(map[string]interface{})
	if !ok || policyValue["status"] != "fail" {
		t.Fatalf("JSON policy = %#v, want failed policy result", document["policy"])
	}
}

func TestDiffPolicyFailureRendersMarkdownAndExitsTwo(t *testing.T) {
	out, serr, code := run(t, "diff",
		"--before", fixture(t, "example.plan.json"),
		"--after", fixture(t, "static_eip_plan.json"),
		"--format", "markdown", "--no-cache", "--timeout", "1s",
		"--fail-on-skipped")
	if code != 2 {
		t.Fatalf("diff policy failure exit code = %d, want 2; stderr=%s", code, serr)
	}
	if !strings.Contains(out, "Cost policy: FAIL") || !strings.Contains(out, "skipped resource") {
		t.Fatalf("markdown did not include policy violation:\n%s", out)
	}
}

func TestInvalidPolicyThresholdExitsOneBeforePricing(t *testing.T) {
	_, serr, code := run(t, "breakdown",
		"--path", fixture(t, "example.plan.json"),
		"--format", "json", "--max-total", "USD=invalid")
	if code != 1 {
		t.Fatalf("invalid policy exit code = %d, want 1", code)
	}
	if !strings.Contains(serr, "load cost policy") {
		t.Fatalf("invalid policy error = %q", serr)
	}
}
