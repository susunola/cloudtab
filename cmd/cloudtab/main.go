// Package main is the cloudtab CLI entrypoint.
//
// Usage:
//
//	cloudtab breakdown --path plan.json --region ap-guangzhou
//	cloudtab diff --before plan.old.json --after plan.new.json [--format markdown]
//
// Auth: reads TENCENTCLOUD_SECRET_ID / TENCENTCLOUD_SECRET_KEY from env.
package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/susunola/cloudtab/internal/output"
	"github.com/susunola/cloudtab/internal/parser"
	"github.com/susunola/cloudtab/internal/pricing"
	"github.com/susunola/cloudtab/internal/resources"
)

func main() {
	root := &cobra.Command{
		Use:   "cloudtab",
		Short: "Tencent Cloud cost estimation from Terraform plans",
	}

	// -- breakdown --
	var (
		path   string
		region string
		format string
	)
	breakdown := &cobra.Command{
		Use:   "breakdown",
		Short: "Show monthly cost of a Terraform plan",
		RunE: func(_ *cobra.Command, _ []string) error {
			engine, err := newEngine(region)
			if err != nil {
				return err
			}
			defer engine.Close()
			rep, err := priceReport(engine, path)
			if err != nil {
				return err
			}
			return output.Render(os.Stdout, rep, format)
		},
	}
	breakdown.Flags().StringVar(&path, "path", "plan.json", "Path to terraform plan.json")
	breakdown.Flags().StringVar(&region, "region", "ap-guangzhou", "Default region for pricing lookup")
	breakdown.Flags().StringVar(&format, "format", "table", "Output format: table|json")

	// -- diff --
	var (
		before  string
		after   string
		diffFmt string
		diffReg string
	)
	diff := &cobra.Command{
		Use:   "diff",
		Short: "Compare monthly cost between two plans (before -> after)",
		RunE: func(_ *cobra.Command, _ []string) error {
			engine, err := newEngine(diffReg)
			if err != nil {
				return err
			}
			defer engine.Close()
			b, err := priceReport(engine, before)
			if err != nil {
				return fmt.Errorf("before: %w", err)
			}
			a, err := priceReport(engine, after)
			if err != nil {
				return fmt.Errorf("after: %w", err)
			}
			return output.RenderDiff(os.Stdout, output.ComputeDiff(b, a), diffFmt)
		},
	}
	diff.Flags().StringVar(&before, "before", "", "Path to baseline plan.json (required)")
	diff.Flags().StringVar(&after, "after", "", "Path to new plan.json (required)")
	diff.Flags().StringVar(&diffReg, "region", "ap-guangzhou", "Default region for pricing lookup")
	diff.Flags().StringVar(&diffFmt, "format", "table", "Output format: table|json|markdown")
	_ = diff.MarkFlagRequired("before")
	_ = diff.MarkFlagRequired("after")

	root.AddCommand(breakdown, diff)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newEngine(region string) (*pricing.Engine, error) {
	return pricing.NewEngine(pricing.Config{
		SecretID:  os.Getenv("TENCENTCLOUD_SECRET_ID"),
		SecretKey: os.Getenv("TENCENTCLOUD_SECRET_KEY"),
		Region:    region,
		CachePath: os.ExpandEnv("$HOME/.cloudtab/cache.db"),
	})
}

// priceReport is the shared pipeline: parse plan -> dispatch to mappers -> collect cost.
// Mappers implementing resources.StaticMapper are evaluated locally; all others are
// routed through the pricing engine with bounded concurrency so we stay under
// Tencent Cloud's InquiryPrice QPS limit.
func priceReport(engine *pricing.Engine, path string) (output.Report, error) {
	var rep output.Report
	plan, err := parser.LoadPlanJSON(path)
	if err != nil {
		return rep, fmt.Errorf("parse plan: %w", err)
	}
	registry := resources.DefaultRegistry()

	const maxConcurrency = 8
	sem := make(chan struct{}, maxConcurrency)
	var mu sync.Mutex
	var g errgroup.Group

	for _, r := range plan.Resources {
		r := r
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()

			cost, skip, err := priceResource(engine, registry, r)
			if err != nil {
				return err
			}
			mu.Lock()
			if cost != nil {
				rep.Resources = append(rep.Resources, *cost)
			}
			if skip != nil {
				rep.Skipped = append(rep.Skipped, *skip)
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return rep, err
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
