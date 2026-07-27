// E2E price accuracy test orchestrator for cloudtab Tencent Cloud products.
//
// Usage:
//
//	go run run.go --all
//	go run run.go --products=cvm,mysql
//	go run run.go --products=cvm --skip-terraform
//
// Requires: TENCENTCLOUD_SECRET_ID, TENCENTCLOUD_SECRET_KEY env vars.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
	"github.com/susunola/cloudtab/internal/resources"
)

// rawPriceRecord saves one resource's complete request-response chain.
type rawPriceRecord struct {
	ResourceAddress string                 `json:"resource_address"`
	ResourceType    string                 `json:"resource_type"`
	Timestamp       string                 `json:"timestamp"`
	Request         map[string]interface{} `json:"request"`
	Response        rawResponse            `json:"response"`
	Error           string                 `json:"error,omitempty"`
}

type rawResponse struct {
	Raw         string `json:"raw"`
	PriceFields string `json:"price_fields,omitempty"` // pretty-printed extracted fields
}

// cloudtabPriceRecord saves cloudtab's parsed CostComponents.
type cloudtabPriceRecord struct {
	Address    string                 `json:"address"`
	Type       string                 `json:"type"`
	Components []output.CostComponent `json:"components"`
}

// productResult aggregates one product's test outcome.
type productResult struct {
	Name      string
	Resources int
	APICalls  int
	Checks    []CheckResult
	Status    string // ALL_PASS | HAS_SUSPICIOUS | HAS_FAIL | API_ERROR | MANUAL | SKIP
}

func main() {
	var (
		productList   string
		allProducts   bool
		skipTerraform bool
		discover      bool
		discoverZone  string
	)
	flag.StringVar(&productList, "products", "", "comma-separated product names (e.g. cvm,mysql)")
	flag.BoolVar(&allProducts, "all", false, "run all products")
	flag.BoolVar(&skipTerraform, "skip-terraform", false, "skip terraform plan, reuse existing plan.json")
	flag.BoolVar(&discover, "discover", false, "print live sellable specs for the given products (postgresql,mongodb) and exit")
	flag.StringVar(&discoverZone, "zone", "ap-guangzhou-3", "availability zone used by --discover")
	flag.Parse()

	if discover {
		runDiscover(strings.Split(productList, ","), "ap-guangzhou", discoverZone)
		return
	}

	if !allProducts && productList == "" {
		fmt.Fprintln(os.Stderr, "usage: go run run.go --all | --products=cvm,mysql [--skip-terraform]")
		os.Exit(2)
	}

	// Credentials
	secretID := os.Getenv("TENCENTCLOUD_SECRET_ID")
	secretKey := os.Getenv("TENCENTCLOUD_SECRET_KEY")

	// Initialize engine (only needed for non-static products; but we create it
	// unconditionally since most products need it).
	var engine *pricing.Engine
	if secretID != "" && secretKey != "" {
		var err error
		engine, err = pricing.NewEngine(pricing.Config{
			SecretID:  secretID,
			SecretKey: secretKey,
			Region:    "ap-guangzhou",
			NoCache:   true, // DESIGN: must use NoCache to get real-time prices
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "engine init: %v\n", err)
			os.Exit(1)
		}
		defer engine.Close()
	} else {
		fmt.Fprintln(os.Stderr, "warning: TENCENTCLOUD_SECRET_ID/KEY not set — only static mappers will work")
	}

	// Select products
	testCases := allTestCases()
	selected := selectProducts(testCases, allProducts, productList)

	// Run tests
	var results []productResult
	for _, tc := range selected {
		result := runProduct(tc, engine, skipTerraform)
		results = append(results, result)
	}

	// Print summary
	hasFail := printSummary(results)
	if hasFail {
		os.Exit(1)
	}
}

// selectProducts filters test cases based on --all or --products flags.
func selectProducts(all []TestCase, runAll bool, csv string) []TestCase {
	if runAll {
		return all
	}
	wanted := map[string]bool{}
	for _, p := range strings.Split(csv, ",") {
		wanted[strings.TrimSpace(p)] = true
	}
	var out []TestCase
	for _, tc := range all {
		if wanted[tc.Name] {
			out = append(out, tc)
		}
	}
	return out
}

// runProduct executes the E2E test for one product.
func runProduct(tc TestCase, engine *pricing.Engine, skipTerraform bool) productResult {
	dir := tc.Name
	result := productResult{Name: tc.Name, Status: "ALL_PASS"}

	// Step 1: Ensure plan.json exists
	planPath := filepath.Join(dir, "plan.json")
	if !skipTerraform {
		if err := ensurePlan(dir); err != nil {
			if isUnsupportedResource(err) {
				result.Status = "MANUAL"
				fmt.Printf("  ⏭ %s: manual test required (TF provider: %v)\n", tc.Name, err)
				return result
			}
			result.Status = "SKIP"
			fmt.Printf("  ⚠ %s: terraform plan failed: %v\n", tc.Name, err)
			return result
		}
	}

	if _, err := os.Stat(planPath); err != nil {
		result.Status = "SKIP"
		fmt.Printf("  ⚠ %s: plan.json not found (run without --skip-terraform)\n", tc.Name)
		return result
	}

	// Step 2: Load plan.json
	plan, err := parser.LoadPlanJSON(planPath)
	if err != nil {
		result.Status = "SKIP"
		fmt.Printf("  ⚠ %s: load plan: %v\n", tc.Name, err)
		return result
	}

	// Filter resources by type
	var resList []parser.PlannedResource
	for _, r := range plan.Resources {
		if r.Type == tc.ResourceType {
			resList = append(resList, r)
		}
	}
	if len(resList) == 0 {
		result.Status = "SKIP"
		fmt.Printf("  ⚠ %s: no %s resources in plan.json\n", tc.Name, tc.ResourceType)
		return result
	}
	result.Resources = len(resList)

	// Step 3: For each resource, run Extract→Query→Parse (or Estimate)
	registry := resources.DefaultRegistry()
	var rawRecords []rawPriceRecord
	var cloudtabRecords []cloudtabPriceRecord
	var allChecks []CheckResult

	for _, r := range resList {
		fmt.Printf("  → %s ... ", r.Address)

		mapper, ok := registry.Lookup(r.Type)
		if !ok {
			fmt.Println("SKIP (no mapper)")
			continue
		}

		// StaticMapper path (EIP)
		if sm, ok := mapper.(resources.StaticMapper); ok {
			comps, err := sm.Estimate(r)
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
				result.Status = "API_ERROR"
				continue
			}
			ctRec := cloudtabPriceRecord{Address: r.Address, Type: r.Type, Components: comps}
			cloudtabRecords = append(cloudtabRecords, ctRec)
			checks := tc.Validator.Validate(pricing.PriceRequest{}, nil, comps)
			allChecks = append(allChecks, checks...)
			printChecks(checks)
			fmt.Println()
			continue
		}

		// Dynamic mapper path: Extract → Query → Parse
		req, err := mapper.Extract(r)
		if err != nil {
			fmt.Printf("Extract ERROR: %v\n", err)
			result.Status = "API_ERROR"
			continue
		}

		if engine == nil {
			fmt.Println("SKIP (no credentials)")
			result.Status = "SKIP"
			continue
		}

		raw, err := engine.Query(req)
		if err != nil {
			fmt.Printf("Query ERROR: %v\n", err)
			rawRecords = append(rawRecords, rawPriceRecord{
				ResourceAddress: r.Address,
				ResourceType:    r.Type,
				Timestamp:       time.Now().Format(time.RFC3339),
				Request:         flattenRequest(req),
				Error:           err.Error(),
			})
			result.Status = "API_ERROR"
			continue
		}
		result.APICalls++

		comps, err := mapper.Parse(req, raw)
		if err != nil {
			fmt.Printf("Parse ERROR: %v\n", err)
			result.Status = "API_ERROR"
			continue
		}

		// Save records
		rawRecords = append(rawRecords, rawPriceRecord{
			ResourceAddress: r.Address,
			ResourceType:    r.Type,
			Timestamp:       time.Now().Format(time.RFC3339),
			Request:         flattenRequest(req),
			Response: rawResponse{
				Raw: string(raw),
			},
		})
		cloudtabRecords = append(cloudtabRecords, cloudtabPriceRecord{
			Address: r.Address, Type: r.Type, Components: comps,
		})

		// Validate
		checks := tc.Validator.Validate(req, raw, comps)
		allChecks = append(allChecks, checks...)
		printChecks(checks)
		fmt.Println()
	}
	result.Checks = allChecks

	// Update status based on checks
	for _, c := range allChecks {
		switch c.Status {
		case "FAIL":
			result.Status = "HAS_FAIL"
		case "SUSPICIOUS":
			if result.Status != "HAS_FAIL" {
				result.Status = "HAS_SUSPICIOUS"
			}
		}
	}

	// Step 4: Save output files
	saveJSON(filepath.Join(dir, "raw_price.json"), rawRecords)
	saveJSON(filepath.Join(dir, "cloudtab_price.json"), cloudtabRecords)

	// Step 5: Generate report.md
	generateReport(dir, tc, resList, rawRecords, cloudtabRecords, allChecks)

	return result
}

// ensurePlan runs terraform init/plan/show to generate plan.json.
func ensurePlan(dir string) error {
	// Check if main.tf exists
	if _, err := os.Stat(filepath.Join(dir, "main.tf")); err != nil {
		return fmt.Errorf("main.tf not found in %s", dir)
	}

	// terraform init (if .terraform doesn't exist)
	if _, err := os.Stat(filepath.Join(dir, ".terraform")); err != nil {
		cmd := exec.Command("terraform", "init")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("terraform init: %w\n%s", err, out)
		}
	}

	// terraform plan (cmd.Dir=dir, so use relative path for -out)
	cmd := exec.Command("terraform", "plan", "-out=tf.plan", "-no-color")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", out)
	}

	// terraform show -json (also runs in dir)
	cmd = exec.Command("terraform", "show", "-json", "tf.plan")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("terraform show: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "plan.json"), out, 0644)
}

// isUnsupportedResource checks if the terraform error is about unsupported resource type.
func isUnsupportedResource(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Unsupported resource type") ||
		strings.Contains(msg, "has not been declared in the provider") ||
		strings.Contains(msg, "Unknown resource") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist")
}

// flattenRequest converts a PriceRequest to a map for JSON serialization.
func flattenRequest(req pricing.PriceRequest) map[string]interface{} {
	return map[string]interface{}{
		"product": req.Product,
		"action":  req.Action,
		"region":  req.Region,
		"params":  req.Params,
	}
}

// saveJSON writes data as pretty-printed JSON to a file.
func saveJSON(path string, data interface{}) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal %s: %v\n", path, err)
		return
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
	}
}

// printChecks prints check results inline.
func printChecks(checks []CheckResult) {
	for _, c := range checks {
		switch c.Status {
		case "PASS":
			fmt.Printf("✅ ")
		case "SUSPICIOUS":
			fmt.Printf("⚠️ ")
		case "FAIL":
			fmt.Printf("❌ ")
		}
	}
}

// generateReport writes the report.md for one product.
func generateReport(dir string, tc TestCase, resList []parser.PlannedResource, rawRecs []rawPriceRecord, ctRecs []cloudtabPriceRecord, checks []CheckResult) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s 价格验证报告\n\n", strings.ToUpper(tc.Name)))
	b.WriteString(fmt.Sprintf("> 生成时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("> 资源类型: %s\n\n", tc.ResourceType))

	// Test resources
	b.WriteString("## 测试资源\n\n")
	b.WriteString("| 资源地址 | 类型 |\n|---|---|\n")
	for _, r := range resList {
		b.WriteString(fmt.Sprintf("| %s | %s |\n", r.Address, r.Type))
	}

	// API raw responses
	if len(rawRecs) > 0 {
		b.WriteString("\n## API 原始返回\n\n")
		for _, rec := range rawRecs {
			b.WriteString(fmt.Sprintf("### %s\n\n", rec.ResourceAddress))
			if rec.Error != "" {
				b.WriteString(fmt.Sprintf("**Error**: %s\n\n", rec.Error))
				continue
			}
			// Pretty-print the raw response
			var pretty interface{}
			if err := json.Unmarshal([]byte(rec.Response.Raw), &pretty); err == nil {
				pb, _ := json.MarshalIndent(pretty, "", "  ")
				b.WriteString("```json\n")
				b.WriteString(string(pb))
				b.WriteString("\n```\n\n")
			}
		}
	}

	// cloudtab results
	if len(ctRecs) > 0 {
		b.WriteString("## cloudtab 计算结果\n\n")
		b.WriteString("| 资源 | 组件 | 小时费 | 月费 | 币种 |\n|---|---|---|---|---|\n")
		for _, rec := range ctRecs {
			for _, c := range rec.Components {
				b.WriteString(fmt.Sprintf("| %s | %s | %.4f | %.2f | %s |\n", rec.Address, c.Name, c.HourlyCost, c.MonthlyCost, c.Currency))
			}
		}
	}

	// Validation checks
	if len(checks) > 0 {
		b.WriteString("\n## 验证\n\n")
		b.WriteString("| 检查项 | API 值 | cloudtab 值 | 计算 | 结果 |\n|---|---|---|---|---|\n")
		for _, c := range checks {
			emoji := "✅"
			switch c.Status {
			case "SUSPICIOUS":
				emoji = "⚠️"
			case "FAIL":
				emoji = "❌"
			}
			b.WriteString(fmt.Sprintf("| %s | %.4f | %.4f | %s | %s %s |\n", c.Name, c.APIValue, c.GotValue, c.Formula, emoji, c.Status))
		}
	}

	// Conclusion
	b.WriteString("\n## 结论\n\n")
	hasFail, hasSusp := false, false
	for _, c := range checks {
		if c.Status == "FAIL" {
			hasFail = true
		}
		if c.Status == "SUSPICIOUS" {
			hasSusp = true
		}
	}
	switch {
	case hasFail:
		b.WriteString("❌ 有检查项未通过 — 请检查 cloudtab 的 Parse 逻辑。\n")
	case hasSusp:
		b.WriteString("⚠️ 有可疑结果 (API 价格为 0) — 请检查 Response 包装层解析。\n")
	default:
		b.WriteString("✅ 全部通过 — cloudtab 对 " + tc.Name + " 的价格查询和计算准确。\n")
	}

	os.WriteFile(filepath.Join(dir, "report.md"), []byte(b.String()), 0644)
}

// printSummary prints the final summary table and returns true if any FAIL.
func printSummary(results []productResult) bool {
	fmt.Println("\n" + strings.Repeat("─", 80))
	fmt.Printf("%-16s %8s %8s %6s %6s %6s   %s\n", "PRODUCT", "RES", "API", "PASS", "SUSP", "FAIL", "STATUS")
	fmt.Println(strings.Repeat("─", 80))

	totalRes, totalAPI, totalPass, totalSusp, totalFail := 0, 0, 0, 0, 0
	hasFail := false
	for _, r := range results {
		pass, susp, fail := 0, 0, 0
		for _, c := range r.Checks {
			switch c.Status {
			case "PASS":
				pass++
			case "SUSPICIOUS":
				susp++
			case "FAIL":
				fail++
			}
		}
		totalRes += r.Resources
		totalAPI += r.APICalls
		totalPass += pass
		totalSusp += susp
		totalFail += fail
		if fail > 0 {
			hasFail = true
		}

		statusEmoji := "✅"
		switch r.Status {
		case "HAS_SUSPICIOUS":
			statusEmoji = "⚠️"
		case "HAS_FAIL":
			statusEmoji = "❌"
		case "API_ERROR":
			statusEmoji = "🔴"
		case "MANUAL":
			statusEmoji = "⏭"
		case "SKIP":
			statusEmoji = "⏭"
		}
		if r.Status == "MANUAL" || r.Status == "SKIP" {
			fmt.Printf("%-16s %8s %8s %6s %6s %6s   %s %s\n", r.Name, "-", "-", "-", "-", "-", statusEmoji, r.Status)
		} else {
			fmt.Printf("%-16s %8d %8d %6d %6d %6d   %s %s\n", r.Name, r.Resources, r.APICalls, pass, susp, fail, statusEmoji, r.Status)
		}
	}
	fmt.Println(strings.Repeat("─", 80))
	fmt.Printf("%-16s %8d %8d %6d %6d %6d\n", "TOTAL", totalRes, totalAPI, totalPass, totalSusp, totalFail)
	return hasFail
}
