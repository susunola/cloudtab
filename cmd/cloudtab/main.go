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
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
	"github.com/susunola/cloudtab/internal/resources"
)

const maxConcurrency = 8

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "(dev)"

func main() {
	root := &cobra.Command{
		Use:     "cloudtab",
		Short:   "Tencent Cloud cost estimation from Terraform plans",
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
		path      string
		region    string
		format    string
		usageFile string
		noCache   bool
		cacheDir  string
		site      string
	)
	breakdown := &cobra.Command{
		Use:   "breakdown",
		Short: "Show monthly cost of a Terraform plan",
		RunE: func(_ *cobra.Command, _ []string) error {
			engine, err := newEngine(region, site, noCache, cacheDir)
			if err != nil {
				return err
			}
			defer engine.Close()

			usage, err := parser.LoadUsageYAML(usageFile)
			if err != nil {
				return fmt.Errorf("load usage file: %w", err)
			}
			rep, err := priceReport(engine, path, usage)
			if err != nil {
				return err
			}
			return output.Render(os.Stdout, rep, format)
		},
	}
	breakdown.Flags().StringVar(&path, "path", "plan.json", "Path to terraform plan.json")
	breakdown.Flags().StringVar(&region, "region", "ap-guangzhou", "Default region for pricing lookup")
	breakdown.Flags().StringVar(&usageFile, "usage-file", "", "Path to usage.yml assumptions (optional)")
	breakdown.Flags().StringVar(&format, "format", "table", "Output format: table|json")
	breakdown.Flags().BoolVar(&noCache, "no-cache", false, "Disable on-disk price cache")
	breakdown.Flags().StringVar(&cacheDir, "cache-dir", "", "Directory for price cache (default $HOME/.cloudtab)")
	breakdown.Flags().StringVar(&site, "site", "", "Tencent Cloud site matching your credential: domestic|intl (default domestic, or $TENCENTCLOUD_SITE)")

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
	)
	diff := &cobra.Command{
		Use:   "diff",
		Short: "Compare monthly cost between two plans (before -> after)",
		RunE: func(_ *cobra.Command, _ []string) error {
			engine, err := newEngine(diffReg, diffSite, diffNoCache, diffCacheDir)
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

			b, err := priceReport(engine, before, beforeUsage)
			if err != nil {
				return fmt.Errorf("before: %w", err)
			}
			a, err := priceReport(engine, after, afterUsage)
			if err != nil {
				return fmt.Errorf("after: %w", err)
			}
			return output.RenderDiff(os.Stdout, output.ComputeDiff(b, a), diffFmt)
		},
	}
	diff.Flags().StringVar(&before, "before", "", "Path to baseline plan.json (required)")
	diff.Flags().StringVar(&after, "after", "", "Path to new plan.json (required)")
	diff.Flags().StringVar(&diffReg, "region", "ap-guangzhou", "Default region for pricing lookup")
	diff.Flags().StringVar(&beforeUsageFile, "before-usage-file", "", "Path to usage.yml for --before plan (optional)")
	diff.Flags().StringVar(&afterUsageFile, "after-usage-file", "", "Path to usage.yml for --after plan (optional)")
	diff.Flags().StringVar(&diffFmt, "format", "table", "Output format: table|json|markdown")
	diff.Flags().BoolVar(&diffNoCache, "no-cache", false, "Disable on-disk price cache")
	diff.Flags().StringVar(&diffCacheDir, "cache-dir", "", "Directory for price cache (default $HOME/.cloudtab)")
	diff.Flags().StringVar(&diffSite, "site", "", "Tencent Cloud site matching your credential: domestic|intl (default domestic, or $TENCENTCLOUD_SITE)")
	_ = diff.MarkFlagRequired("before")
	_ = diff.MarkFlagRequired("after")

	root.AddCommand(breakdown, diff)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newEngine(region, site string, noCache bool, cacheDir string) (*pricing.Engine, error) {
	return pricing.NewEngine(pricing.Config{
		SecretID:  os.Getenv("TENCENTCLOUD_SECRET_ID"),
		SecretKey: os.Getenv("TENCENTCLOUD_SECRET_KEY"),
		Region:    region,
		Site:      resolveSite(site),
		CachePath: cachePathForFlags(noCache, cacheDir),
		NoCache:   noCache,

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
// Tencent Cloud's InquiryPrice QPS limit.
func priceReport(engine *pricing.Engine, path string, usage parser.UsageOverrides) (output.Report, error) {
	var rep output.Report
	plan, err := parser.LoadPlanJSON(path)
	if err != nil {
		return rep, fmt.Errorf("parse plan: %w", err)
	}
	registry := resources.DefaultRegistry()

	jobs := make(chan parser.PlannedResource, len(plan.Resources))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make(chan error, maxConcurrency)

	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				cost, skip, err := priceResource(engine, registry, r)
				if err != nil {
					errs <- fmt.Errorf("%s: %w", r.Address, err)
					continue
				}
				mu.Lock()
				if cost != nil {
					rep.Resources = append(rep.Resources, *cost)
				}
				if skip != nil {
					rep.Skipped = append(rep.Skipped, *skip)
				}
				mu.Unlock()
			}
		}()
	}

	for _, r := range plan.Resources {
		if u, ok := usage[r.Address]; ok {
			r = mergeUsageIntoAfter(r, u)
		}
		jobs <- r
	}
	close(jobs)
	wg.Wait()
	close(errs)

	var pricingErrs []error
	for e := range errs {
		pricingErrs = append(pricingErrs, e)
	}
	if len(pricingErrs) > 0 {
		return rep, errors.Join(pricingErrs...)
	}
	return rep, nil
}

// priceResource prices a single planned resource, returning either a cost line
// or a skip reason. A non-nil error is a hard failure (e.g. API error) that
// aborts the whole report.
func priceResource(engine *pricing.Engine, registry *resources.Registry, r parser.PlannedResource) (*output.ResourceCost, *output.SkippedResource, error) {
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
		return nil, nil, fmt.Errorf("query %s: %w", r.Address, err)
	}
	comps, err := mapper.Parse(req, raw)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", r.Address, err)
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
