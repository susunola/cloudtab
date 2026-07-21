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

	"github.com/spf13/cobra"

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
		before   string
		after    string
		diffFmt  string
		diffReg  string
	)
	diff := &cobra.Command{
		Use:   "diff",
		Short: "Compare monthly cost between two plans (before → after)",
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

// priceReport is the shared pipeline: parse plan → dispatch to mappers → collect cost.
func priceReport(engine *pricing.Engine, path string) (output.Report, error) {
	var rep output.Report
	plan, err := parser.LoadPlanJSON(path)
	if err != nil {
		return rep, fmt.Errorf("parse plan: %w", err)
	}
	registry := resources.DefaultRegistry()
	for _, r := range plan.Resources {
		mapper, ok := registry.Lookup(r.Type)
		if !ok {
			rep.Skipped = append(rep.Skipped, output.SkippedResource{
				Address: r.Address, Type: r.Type, Reason: "unsupported resource type",
			})
			continue
		}
		req, err := mapper.Extract(r)
		if err != nil {
			rep.Skipped = append(rep.Skipped, output.SkippedResource{
				Address: r.Address, Type: r.Type, Reason: err.Error(),
			})
			continue
		}
		raw, err := engine.Query(req)
		if err != nil {
			return rep, fmt.Errorf("query %s: %w", r.Address, err)
		}
		comps, err := mapper.Parse(req, raw)
		if err != nil {
			return rep, fmt.Errorf("parse %s: %w", r.Address, err)
		}
		rep.Resources = append(rep.Resources, output.ResourceCost{
			Address: r.Address, Type: r.Type, Components: comps,
		})
	}
	return rep, nil
}
