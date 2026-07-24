// Package main is the cloudtab CLI entrypoint.
//
// Usage:
//
//	cloudtab breakdown --path plan.json --region ap-guangzhou
//	cloudtab diff --before plan.old.json --after plan.new.json [--format markdown]
//
// Auth (Tencent Cloud): reads TENCENTCLOUD_SECRET_ID / TENCENTCLOUD_SECRET_KEY
// from env.
//
// Auth (AWS): the AWS Price List backend uses the standard AWS credential chain
// (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN, shared config
// files, IAM role, ...). It is only consulted when the plan contains aws_*
// resources, so a pure-Tencent run needs no AWS credentials.
//
// A single plan may mix tencentcloud_* and aws_* resources; each resource is
// routed to the matching pricing backend by its provider prefix.
//
// Site: Tencent Cloud has two independent sites (Chinese-mainland and
// International) with separate account systems. The site is chosen by the
// credential, not the region, so it is selected explicitly with --site
// (domestic|intl) or the TENCENTCLOUD_SITE env var. Default is domestic.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
	"github.com/susunola/cloudtab/internal/resources"
)

// defaultConcurrency bounds how many pricing lookups run in parallel. It keeps
// us comfortably under Tencent Cloud's InquiryPrice QPS limit while still
// overlapping network latency. Override with --concurrency / $CLOUDTAB_CONCURRENCY.
const defaultConcurrency = 8

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "(dev)"

func main() {
	root := &cobra.Command{
		Use:     "cloudtab",
		Short:   "Multi-cloud cost estimation from Terraform plans",
		Version: Version,
	}
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print cloudtab version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("cloudtab", Version)
		},
	})

	// -- breakdown --
	var (
		path        string
		region      string
		format      string
		usageFile   string
		noCache     bool
		cacheDir    string
		site        string
		concurrency int
		timeout     time.Duration
		maxRetries  int
		failOnError bool
		cacheTTL    time.Duration
	)
	breakdown := &cobra.Command{
		Use:   "breakdown",
		Short: "Show monthly cost of a Terraform plan",
		RunE: func(_ *cobra.Command, _ []string) error {
			engine, err := newEngine(region, site, noCache, cacheDir, timeout, maxRetries, cacheTTL)
			if err != nil {
				return err
			}
			defer engine.Close()

			usage, err := parser.LoadUsageYAML(usageFile)
			if err != nil {
				return fmt.Errorf("load usage file: %w", err)
			}
			rep, err := priceReport(engine, path, usage, resolveConcurrency(concurrency), failOnError)
			if err != nil {
				return err
			}
			return output.Render(os.Stdout, rep, format)
		},
	}
	breakdown.Flags().StringVar(&path, "path", "plan.json", "Path to terraform plan.json")
	breakdown.Flags().StringVar(&region, "region", "ap-guangzhou", "Fallback region for resources whose provider block omits one (applies across providers; example: ap-guangzhou)")
	breakdown.Flags().StringVar(&usageFile, "usage-file", "", "Path to usage.yml assumptions (optional)")
	breakdown.Flags().StringVar(&format, "format", "table", "Output format: table|json (markdown is available via `cloudtab diff`)")
	breakdown.Flags().BoolVar(&noCache, "no-cache", false, "Disable on-disk price cache")
	breakdown.Flags().StringVar(&cacheDir, "cache-dir", "", "Directory for price cache (default $HOME/.cloudtab)")
	breakdown.Flags().StringVar(&site, "site", "", "Tencent Cloud site matching your credential: domestic|intl (default domestic, or $TENCENTCLOUD_SITE)")
	breakdown.Flags().IntVar(&concurrency, "concurrency", 0, "Parallel pricing lookups (default 8, or $CLOUDTAB_CONCURRENCY)")
	breakdown.Flags().DurationVar(&timeout, "timeout", 0, "Per-request pricing timeout (default 30s)")
	breakdown.Flags().IntVar(&maxRetries, "max-retries", 0, "Retries on transient/rate-limit errors (default 2; negative disables)")
	breakdown.Flags().BoolVar(&failOnError, "fail-on-error", false, "Fail the whole report if any resource pricing errors (default: skip failed resources and continue)")
	breakdown.Flags().DurationVar(&cacheTTL, "cache-ttl", 0, "Price cache entry TTL (default 24h)")

	// -- diff --
	var (
		before          string
		after           string
		diffFmt         string
		diffReg         string
		beforeUsageFile string
		afterUsageFile  string
		diffNoCache     bool
		diffCacheDir    string
		diffSite        string
		diffConcurrency int
		diffTimeout     time.Duration
		diffMaxRetries  int
		diffFailOnError bool
		diffCacheTTL    time.Duration
	)
	diff := &cobra.Command{
		Use:   "diff",
		Short: "Compare monthly cost between two plans (before -> after)",
		RunE: func(_ *cobra.Command, _ []string) error {
			engine, err := newEngine(diffReg, diffSite, diffNoCache, diffCacheDir, diffTimeout, diffMaxRetries, diffCacheTTL)
			if err != nil {
				return err
			}
			defer engine.Close()

			beforeUsage, err := parser.LoadUsageYAML(beforeUsageFile)
			if err != nil {
				return fmt.Errorf("load before usage file: %w", err)
			}
			afterUsage, err := parser.LoadUsageYAML(afterUsageFile)
			if err != nil {
				return fmt.Errorf("load after usage file: %w", err)
			}

			conc := resolveConcurrency(diffConcurrency)
			b, err := priceReport(engine, before, beforeUsage, conc, diffFailOnError)
			if err != nil {
				return fmt.Errorf("before: %w", err)
			}
			a, err := priceReport(engine, after, afterUsage, conc, diffFailOnError)
			if err != nil {
				return fmt.Errorf("after: %w", err)
			}
			return output.RenderDiff(os.Stdout, output.ComputeDiff(b, a), diffFmt)
		},
	}
	diff.Flags().StringVar(&before, "before", "", "Path to baseline plan.json (required)")
	diff.Flags().StringVar(&after, "after", "", "Path to new plan.json (required)")
	diff.Flags().StringVar(&diffReg, "region", "ap-guangzhou", "Fallback region for resources whose provider block omits one (applies across providers; example: ap-guangzhou)")
	diff.Flags().StringVar(&beforeUsageFile, "before-usage-file", "", "Path to usage.yml for --before plan (optional)")
	diff.Flags().StringVar(&afterUsageFile, "after-usage-file", "", "Path to usage.yml for --after plan (optional)")
	diff.Flags().StringVar(&diffFmt, "format", "table", "Output format: table|json|markdown")
	diff.Flags().BoolVar(&diffNoCache, "no-cache", false, "Disable on-disk price cache")
	diff.Flags().StringVar(&diffCacheDir, "cache-dir", "", "Directory for price cache (default $HOME/.cloudtab)")
	diff.Flags().StringVar(&diffSite, "site", "", "Tencent Cloud site matching your credential: domestic|intl (default domestic, or $TENCENTCLOUD_SITE)")
	diff.Flags().IntVar(&diffConcurrency, "concurrency", 0, "Parallel pricing lookups (default 8, or $CLOUDTAB_CONCURRENCY)")
	diff.Flags().DurationVar(&diffTimeout, "timeout", 0, "Per-request pricing timeout (default 30s)")
	diff.Flags().IntVar(&diffMaxRetries, "max-retries", 0, "Retries on transient/rate-limit errors (default 2; negative disables)")
	diff.Flags().BoolVar(&diffFailOnError, "fail-on-error", false, "Fail the whole report if any resource pricing errors (default: skip failed resources and continue)")
	diff.Flags().DurationVar(&diffCacheTTL, "cache-ttl", 0, "Price cache entry TTL (default 24h)")
	_ = diff.MarkFlagRequired("before")
	_ = diff.MarkFlagRequired("after")

	root.AddCommand(breakdown, diff)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newEngine(region, site string, noCache bool, cacheDir string, timeout time.Duration, maxRetries int, cacheTTL time.Duration) (*pricing.Engine, error) {
	return pricing.NewEngine(pricing.Config{
		SecretID:  os.Getenv("TENCENTCLOUD_SECRET_ID"),
		SecretKey: os.Getenv("TENCENTCLOUD_SECRET_KEY"),
		Region:    region,
		Site:      resolveSite(site),
		CachePath: cachePathForFlags(noCache, cacheDir),
		NoCache:   noCache,

		// Per-request timeout and transient-error retry budget. Zero values let
		// the engine apply its own defaults (30s / 2 retries), so passing the
		// flag's zero-value default here is intentional and safe.
		Timeout:    timeout,
		MaxRetries: maxRetries,
		CacheTTL:   cacheTTL,

		// Huawei Cloud BSS project id (a UUID, NOT a region), used as
		// RateOnDemandReq.ProjectId. Optional; read from HUAWEI_PROJECT_ID.
		HuaweiProjectID: os.Getenv("HUAWEI_PROJECT_ID"),

		// AWS credentials for the optional AWS Price List backend. These are
		// read from the standard AWS environment variables and are only used
		// when the plan contains aws_* resources; a pure-Tencent run ignores
		// them entirely (the AWS backend is created lazily on first use).
		AWSAccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSSessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
	})
}

// resolveSite picks the Tencent Cloud site with an explicit precedence: the
// --site flag wins; otherwise fall back to the TENCENTCLOUD_SITE env var;
// otherwise empty (which the engine treats as the Chinese-mainland default).
func resolveSite(flagSite string) string {
	if s := strings.TrimSpace(flagSite); s != "" {
		return s
	}
	return strings.TrimSpace(os.Getenv("TENCENTCLOUD_SITE"))
}

// resolveConcurrency picks the parallel-lookup count with an explicit
// precedence: a positive --concurrency flag wins; otherwise a positive
// $CLOUDTAB_CONCURRENCY env var; otherwise the built-in default. Non-positive
// or unparseable values fall through to the next source so a bad env var never
// silently drops the pipeline to a crawl.
func resolveConcurrency(flagVal int) int {
	if flagVal > 0 {
		return flagVal
	}
	if s := strings.TrimSpace(os.Getenv("CLOUDTAB_CONCURRENCY")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return defaultConcurrency
}

func cachePathForFlags(noCache bool, cacheDir string) string {
	if noCache {
		return ""
	}
	if cacheDir == "" {
		cacheDir = os.ExpandEnv("$HOME/.cloudtab")
	}
	return filepath.Join(cacheDir, "cache.db")
}

// priceReport is the shared pipeline: parse plan -> dispatch to mappers -> collect cost.
// Mappers implementing resources.StaticMapper are evaluated locally; all others are
// routed through the pricing engine with bounded concurrency so we stay under
// the provider's pricing QPS limit.
//
// By default the run is lenient: a resource whose pricing errors (API failure,
// parse failure) is recorded as a SkippedResource and the rest of the report
// still renders, so a single bad SKU cannot sink an entire large plan (code
// review #5). Pass failOnError=true to restore the old hard-fail behaviour.
//
// Results and errors are drained by a single dedicated collector goroutine
// running concurrently with the workers. This is deliberate: an earlier version
// buffered errors in a channel sized to the worker count and only drained it
// after wg.Wait(), which deadlocked when more resources failed than there were
// workers (every worker blocked writing to the full error channel while nobody
// was reading). A live collector removes that failure mode entirely regardless
// of how many resources error.
func priceReport(engine *pricing.Engine, path string, usage parser.UsageOverrides, concurrency int, failOnError bool) (output.Report, error) {
	var rep output.Report
	plan, err := parser.LoadPlanJSON(path)
	if err != nil {
		return rep, fmt.Errorf("parse plan: %w (hint: pass the JSON form — run 'terraform show -json <planfile> > plan.json')", err)
	}
	registry := resources.DefaultRegistry()

	if concurrency < 1 {
		concurrency = 1
	}
	// No point spinning up more workers than there are resources to price.
	if n := len(plan.Resources); n > 0 && concurrency > n {
		concurrency = n
	}

	type result struct {
		cost *output.ResourceCost
		skip *output.SkippedResource
		err  error
	}

	jobs := make(chan parser.PlannedResource, len(plan.Resources))
	results := make(chan result, len(plan.Resources))
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				cost, skip, err := priceResource(engine, registry, r, failOnError)
				if err != nil {
					results <- result{err: fmt.Errorf("%s: %w", r.Address, err)}
					continue
				}
				results <- result{cost: cost, skip: skip}
			}
		}()
	}

	// Close results once all workers have finished, so the collector's range
	// terminates. The collector runs concurrently with the workers, so the
	// results channel can never back-pressure a worker into a deadlock.
	go func() {
		wg.Wait()
		close(results)
	}()

	for _, r := range plan.Resources {
		if u, ok := usage[r.Address]; ok {
			r = mergeUsageIntoAfter(r, u)
		}
		jobs <- r
	}
	close(jobs)

	var pricingErrs []error
	for res := range results {
		if res.err != nil {
			pricingErrs = append(pricingErrs, res.err)
			continue
		}
		if res.cost != nil {
			rep.Resources = append(rep.Resources, *res.cost)
		}
		if res.skip != nil {
			rep.Skipped = append(rep.Skipped, *res.skip)
		}
	}

	if len(pricingErrs) > 0 {
		return rep, errors.Join(pricingErrs...)
	}
	return rep, nil
}

// priceResource prices a single planned resource, returning either a cost line
// or a skip reason. When failOnError is false (default), an API or parse error
// becomes a SkippedResource rather than a hard failure, so one bad SKU cannot
// abort the whole report. When failOnError is true, such errors are returned as
// a non-nil error and fail the report.
func priceResource(engine *pricing.Engine, registry *resources.Registry, r parser.PlannedResource, failOnError bool) (*output.ResourceCost, *output.SkippedResource, error) {
	mapper, ok := registry.Lookup(r.Type)
	if !ok {
		return nil, &output.SkippedResource{
			Address: r.Address, Type: r.Type, Reason: "unsupported resource type",
		}, nil
	}

	// Static mappers bypass the pricing engine entirely (e.g. EIP, which has no
	// Tencent InquiryPrice API).
	if sm, ok := mapper.(resources.StaticMapper); ok {
		comps, err := sm.Estimate(r)
		if err != nil {
			return nil, &output.SkippedResource{
				Address: r.Address, Type: r.Type, Reason: err.Error(),
			}, nil
		}
		return &output.ResourceCost{
			Address: r.Address, Type: r.Type, Components: comps,
		}, nil, nil
	}

	req, err := mapper.Extract(r)
	if err != nil {
		return nil, &output.SkippedResource{
			Address: r.Address, Type: r.Type, Reason: err.Error(),
		}, nil
	}
	raw, err := engine.Query(req)
	if err != nil {
		if failOnError {
			return nil, nil, fmt.Errorf("query %s: %w", r.Address, err)
		}
		return nil, &output.SkippedResource{
			Address: r.Address, Type: r.Type, Reason: err.Error(),
		}, nil
	}
	comps, err := mapper.Parse(req, raw)
	if err != nil {
		if failOnError {
			return nil, nil, fmt.Errorf("parse %s: %w", r.Address, err)
		}
		return nil, &output.SkippedResource{
			Address: r.Address, Type: r.Type, Reason: err.Error(),
		}, nil
	}
	return &output.ResourceCost{
		Address: r.Address, Type: r.Type, Components: comps,
	}, nil, nil
}

func mergeUsageIntoAfter(r parser.PlannedResource, usage map[string]interface{}) parser.PlannedResource {
	if len(usage) == 0 {
		return r
	}
	merged := make(map[string]interface{}, len(r.After)+len(usage))
	for k, v := range r.After {
		merged[k] = v
	}
	for k, v := range usage {
		merged[k] = v // usage wins on key conflict
	}
	r.After = merged
	return r
}
